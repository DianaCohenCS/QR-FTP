/*
This is the server program that responds to client's request for file.
The server responds by sending the whole file, frame-by-frame.
Client will only work if the server is already running, the requested file exists on it,
and the server has read access to it.
Server will read a portion of file, generage the QRCode and receive ACK from client.
*/

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/go-transfer-file-qrcode/utils"
)

const DEBUG bool = false // whether to enable PrintDebugMessage
const MIN_BUF_SIZE int = 8
const MAX_BUF_SIZE int = 1024
const MIN_PORT int = 0
const MAX_PORT int = 65535
const MSG_ERR string = "ERR"
const MSG_FROM string = "SERVER"
const MSG_TO string = "CLIENT"

var bufferSize int = 39    // some default buffer-size, don't forget to decrement 1 to save place for seq
var seqToSend int = 0      // sequence number of message to send
var srvAddress string = "" // outbound IP address of the server
var prevMessage string = ""
var webcamPath string = "/dev/video0"

// get the outbound IP address of the server
func getLocalAddress() string {
	utils.PrintDebugMessage("enter getLocalAddress", DEBUG)
	defer utils.PrintDebugMessage("exit getLocalAddress\n", DEBUG)

	utils.PrintDebugMessage("Trying to dial google", DEBUG)
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("Error dialing google:", err)
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// send the message to the client, the message is buffer-bounded for client
func sendMessageToClient(c net.Conn, message string) bool {
	utils.PrintDebugMessage("enter sendMessageToClient", DEBUG)
	defer utils.PrintDebugMessage("exit sendMessageToClient\n", DEBUG)

	return utils.SendMessage(c, message, MSG_FROM, MSG_TO, DEBUG)
}

// receive the message from client
func receiveMessageFromClient(c net.Conn) (bool, string) {
	utils.PrintDebugMessage("enter receiveMessageFromClient", DEBUG)
	defer utils.PrintDebugMessage("exit receiveMessageFromClient\n", DEBUG)

	utils.PrintDebugMessage("Trying to receive message from client", DEBUG)

	// forget about TCP...
	/*
		// receive the message from client in line format and retrieve the corresponding string
		msg, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			fmt.Println("Error receiving message from client:", err)
			return false, MSG_ERR
		}

		msg = strings.TrimSpace(msg)
		fmt.Println("CLIENT:", msg)
		return true, msg
	*/

	// discard same messages and empty messages
	msg := ""
	res := false
	//time.Sleep(utils.WAIT_TIME_RECEIVE)
	for retryIndex := 0; retryIndex < utils.RETRY; retryIndex++ {
		res, msg = utils.ReceiveMessage(MSG_TO, MSG_FROM, prevMessage, webcamPath, DEBUG)
		if res && strings.TrimSpace(msg) != "" { //&& msg != prevMessage {
			break // exit the loop if different message received
		}
		time.Sleep(utils.WAIT_TIME_RECEIVE * time.Duration(retryIndex+1)) // wait before retrying
	}
	if !res {
		fmt.Println("Error receiving message from client")
		return false, MSG_ERR
	}

	if strings.TrimSpace(msg) != "" {
		prevMessage = msg // update the prevMessage
	}
	return res, msg
}

// get the INIT message containing the source file and the buffer-size of a client
func getInitMessageFromClient(c net.Conn) (bool, string, int) {
	utils.PrintDebugMessage("enter getInitMessageFromClient", DEBUG)
	defer utils.PrintDebugMessage("exit getInitMessageFromClient\n", DEBUG)

	time.Sleep(utils.WAIT_TIME_SEND) // get some extra sleep so client can start...

	utils.PrintDebugMessage("Trying to get the INIT message from client", DEBUG)
	res, msg := receiveMessageFromClient(c)
	if !res {
		fmt.Println("Error getting the INIT message from client")
		return false, MSG_ERR, 0
	}

	// split the msg by comma and retrieve source filename and buffer-size
	utils.PrintDebugMessage("Trying to split the message by comma delimiter (SRC, BUF_SIZE)", DEBUG)
	stringSlice := strings.Split(msg, ",")
	if len(stringSlice) != 2 {
		fmt.Println("Bad parameters - must include source filename and buffer-size")
		return false, MSG_ERR, 0
	}

	utils.PrintDebugMessage("Trying to extract the SRC and BUF_SIZE", DEBUG)
	src := strings.TrimSpace(stringSlice[0])
	bufSize, err := strconv.Atoi(strings.TrimSpace(stringSlice[1]))
	if err != nil {
		fmt.Println("Bad parameter - buffer-size must be integer")
		fmt.Println("Error converting string to integer:", err)
		return false, MSG_ERR, 0
	}
	// validate for minimum buffer-size
	if bufSize < MIN_BUF_SIZE || bufSize > MAX_BUF_SIZE {
		fmt.Println("Bad parameter - buffer-size must be in range of", MIN_BUF_SIZE, MAX_BUF_SIZE)
		return false, MSG_ERR, 0
	}

	utils.PrintDebugMessage("Extracted the SRC = "+src+" and BUF_SIZE = "+strconv.Itoa(bufSize), DEBUG)
	return true, src, bufSize
}

// handle the message exchange - send to client and receive ack from client
// sendMessageToClient - encode the message to create QRCode and display it
// receiveMessageFromClient - ack received
// continue to the next message
// no need to re-transmit the current message:
// QRCode will still be displayed and the client will read it again
func exchangeMessages(c net.Conn, msg string) bool {
	utils.PrintDebugMessage("enter exchangeMessages", DEBUG)
	defer utils.PrintDebugMessage("exit exchangeMessages\n", DEBUG)

	message := strconv.Itoa(seqToSend) + msg
	utils.PrintDebugMessage("Actual message to be sent, with the sequence number: "+message, DEBUG)

	// no need to re-transmit the current message:
	// QRCode will still be displayed and the client will read it again
	if !sendMessageToClient(c, message) {
		fmt.Println("Server failed to send message to client")
		return false
	}

	for retryIndex := 0; retryIndex < utils.RETRY; retryIndex++ {
		// retransmit the message
		/*
			if !sendMessageToClient(c, message) {
				fmt.Println("Server failed to send message to client")
				continue
			}
		*/

		res, ack := receiveMessageFromClient(c)
		if !res {
			fmt.Println("Server failed to receive message from client")
			continue // don't return here, try again
		}
		// whenever there is a first ERR message, just break the connection
		if strings.TrimSpace(ack) == "ERR" {
			fmt.Println("Server received ERR message from client")
			return false
		}
		if strings.TrimSpace(ack) == "" {
			continue
		}
		if int(ack[0]) != utils.DIGIT_OFFSET && int(ack[0]) != utils.DIGIT_OFFSET+1 {
			// this isn't the message expected, maybe old QR Code is scanned
			continue
		}

		// ack received:
		ackMsg := ""
		ackSeq := int(ack[0]) - utils.DIGIT_OFFSET // first byte is for sequence number
		if len(ack) > 1 {
			ackMsg = ack[1:] // message itself starts from second byte
		}

		// if arrived seq differs, then just try to receive another one (done by the loop)
		// otherwise, this is expected ack -> proceed
		if ackSeq == seqToSend {
			//if strings.TrimSpace(ackMsg) == "ERR" {
			if strings.TrimSpace(ackMsg) != "ACK" {
				fmt.Println("Server didn't receive expected ACK message from client")
				continue // don't return here, try again
			}
			// increment sequence number to proceed
			seqToSend = utils.IncrementSEQ(seqToSend)
			return true // exit the loop
		}
	}

	// exited the loop after unsuccessful amount of retries
	return false
}

// validate the src filename
func getFile(src string) bool {
	utils.PrintDebugMessage("enter getFile", DEBUG)
	defer utils.PrintDebugMessage("exit getFile\n", DEBUG)

	// check if the path exists
	utils.PrintDebugMessage("Trying to get FileInfo for "+src, DEBUG)
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		fmt.Println("Path does not exist:", err)
		return false
	}

	// check if it is a regular file
	utils.PrintDebugMessage("Trying to get Mode of the source file", DEBUG)
	mode := sourceFileStat.Mode()
	if !mode.IsRegular() {
		fmt.Println("Error:", src, "is not a regular file.")
		return false
	}

	// this is a regular file - can be processed
	fmt.Println("The file can be processed now")
	return true
}

// read the file into buffer and exchange messages with client
// if something went wrong, break the reading loop
func readFileBuffer(c net.Conn, src string) bool {
	utils.PrintDebugMessage("enter readFileBuffer", DEBUG)
	defer utils.PrintDebugMessage("exit readFileBuffer\n", DEBUG)

	utils.PrintDebugMessage("Trying to open source file = "+src, DEBUG)
	source, err := os.Open(src)
	if err != nil {
		fmt.Println("Error openning the source file:", err)
		return false
	}
	utils.PrintDebugMessage("Source file is open", DEBUG)

	// close before exiting the function
	defer source.Close()
	defer fmt.Println("Server stopped reading source file")

	buf := make([]byte, bufferSize)
	for {
		utils.PrintDebugMessage("Trying to read into buffer", DEBUG)
		n, err := source.Read(buf)
		if err == io.EOF {
			fmt.Println("EOF")
			break // this is the End Of File - DONE
		}
		if err != nil {
			fmt.Println("Error reading file:", err)
			return false
		}
		// nothing read - DONE
		if n == 0 {
			fmt.Println("Nothing read into buffer")
			break
		}

		message := string(buf[:n]) // successfully read to buffer
		utils.PrintDebugMessage("Successfully read into buffer: "+message+" ("+strconv.Itoa(n)+" bytes)", DEBUG)

		utils.PrintDebugMessage("Trying to exchange messages with client", DEBUG)
		if !exchangeMessages(c, message) {
			fmt.Println("Server failed to process message transfer")
			return false
		}
		utils.PrintDebugMessage("Successfully exchanged messages with client", DEBUG)
	}

	return true
}

// core function for processing the connection with single client
// establish connection:
// 	display QRCode with IP:PORT
// 	(TCP) get INIT message having source-filename and client buffer-size
// proceed the file transfer till the end or error received by client
// 	use sequence number 0 or 1 at the beginning of the message
func processConnection(l net.Listener, srvAddress string) {
	utils.PrintDebugMessage("enter processConnection", DEBUG)
	defer utils.PrintDebugMessage("exit processConnection\n", DEBUG)

	seqToSend = 0        // sequence number of message to send
	var c net.Conn = nil // nesessary for TCP connection, for QR Codes just leave it nil

	// establish connection
	fmt.Println("Server is waiting for client")

	// create first image with the srvAddress
	utils.PrintDebugMessage("Trying to send INIT message to client", DEBUG)
	if !sendMessageToClient(c, srvAddress) {
		fmt.Println("Server failed sending INIT message to client")
		return
	}

	// it's time to forget about TCP...
	/*
		// this is irrelevant once switch off the TCP
		utils.PrintDebugMessage("Trying to accept the client", DEBUG)
		c, err := l.Accept() // wait for client to connect
		if err != nil {
			fmt.Println("Error accepting the client:", err)
			return
		}
	*/

	//defer time.Sleep(utils.WAIT_TIME_SEND) // wait some time before disconnecting, so the client will have a chance to receive the last message
	defer fmt.Println("Server is disconnecting from client")

	// it's time to forget about TCP...
	/*
		// this is irrelevant once switch off the TCP
		defer c.Close() // don't forget to close the connection
		fmt.Println("Connection with client established")
	*/

	// receive the initial message from client
	utils.PrintDebugMessage("Trying to receive INIT message from client", DEBUG)
	result, source, bufSize := getInitMessageFromClient(c)
	if !result {
		fmt.Println("Server failed to receive the INIT message from client")

		// close connection
		sendMessageToClient(c, "ERR")
		return
	}
	// validate the source file
	if !getFile(source) {
		fmt.Println("Server failed to access the file to transfer")

		// close connection
		sendMessageToClient(c, "ERR")
		return
	}
	// set the buffer-size according to client's
	bufferSize = bufSize - 1 // save the first byte for sequence number

	// process connection
	utils.PrintDebugMessage("Trying to process the source file", DEBUG)
	if !readFileBuffer(c, source) {
		fmt.Println("Server failed to process the file to transfer")

		// close connection
		sendMessageToClient(c, "ERR")
		return
	}

	// successfully read the file - close connection
	fmt.Println("Server finished to transfer the file successfully")
	sendMessageToClient(c, "FIN")
}

func main() {
	utils.PrintDebugMessage("enter main", DEBUG)
	defer utils.PrintDebugMessage("exit main\n", DEBUG)

	utils.PrintDebugMessage("Checking command line arguments", DEBUG)
	args := os.Args
	/*
		if len(args) != 2 {
			fmt.Printf("Usage: %s PORT\n", filepath.Base(args[0]))
			return
		}
		// handle arguments passed
		port, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Println("Bad parameter - port must be integer")
			fmt.Println("Error converting string to integer:", err)
			return
		}
		// validate port number
		if port < MIN_PORT || port > MAX_PORT {
			fmt.Println("Bad parameter - port must be in range of", MIN_PORT, MAX_PORT)
			return
		}
	*/
	if len(args) != 2 {
		fmt.Printf("Usage: %s WEBCAM\n", filepath.Base(args[0]))
		return
	}
	webcamPath = args[1]
	utils.PrintDebugMessage("Command line arguments - OK", DEBUG)

	// server is good to start
	//srvAddress = getLocalAddress() + ":" + args[1] // get the outbound IP:PORT of the server
	srvAddress = getLocalAddress() // get the outbound IP of the server
	fmt.Println("Server is running")

	var l net.Listener = nil // nesessary for TCP connection, for QR Codes just leave it nil

	// it's time to forget about TCP...
	/*
		// listen to the specified port for incomming (single) connection request
		utils.PrintDebugMessage("Trying to listen on "+srvAddress, DEBUG)
		l, err = net.Listen("tcp", srvAddress)
		if err != nil {
			fmt.Println("Server failed to listen:", err)
			fmt.Println("Server is exiting with error...")
			return
		}

		fmt.Println("Server is listening on", srvAddress)
		defer l.Close() // don't forget to close at the end
	*/

	defer fmt.Println("Server is exiting...") // final message

	// loop dealing with consequence clients
	// each iteration deals with single client connection
	//for {
	utils.PrintDebugMessage("Trying to process client", DEBUG)
	processConnection(l, srvAddress)
	//}
}
