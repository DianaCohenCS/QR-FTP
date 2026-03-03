/*
This is the client program that can request a file from server program.
The server responds by sending the whole file.
Client will only work if the server is already running, the requested file exists on it,
and the server has read access to it.
*/

package main

import (
	"fmt"
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
const MAX_BUF_SIZE int = 512
const MSG_ERR string = "ERR"
const MSG_ACK string = "ACK"
const MSG_FROM string = "CLIENT"
const MSG_TO string = "SERVER"

//const MSG_SUFFIX string = "\n" // use \n on TCP connection
const MSG_SUFFIX string = "" // no new line for QR Code connection

var bufferSize int = 40          // this is the default value for number if bytes the buffer can hold
var seqExpected int = 0          // sequence number of the expected message
var ackSeq int = 1 - seqExpected //tell which frame is being acked
var prevMessage string = ""
var webcamPath string = "/dev/video2"

// send the message to the server
func sendMessageToServer(c net.Conn, message string) bool {
	utils.PrintDebugMessage("enter sendMessageToServer", DEBUG)
	defer utils.PrintDebugMessage("exit sendMessageToServer\n", DEBUG)

	return utils.SendMessage(c, message, MSG_FROM, MSG_TO, DEBUG)
}

// receive the message from server
func receiveMessageFromServer(c net.Conn, buf []byte) (bool, string) {
	utils.PrintDebugMessage("enter receiveMessageFromServer", DEBUG)
	defer utils.PrintDebugMessage("exit receiveMessageFromServer\n", DEBUG)

	utils.PrintDebugMessage("Trying to receive message from server", DEBUG)
	/*
		// forget about TCP...
		// receive the message from the server into a buffer and return a corresponding string message
		n, err := c.Read(buf) // receive message from server into a buffer
		if err == io.EOF {
			// this is the End Of File - DONE
			fmt.Println("EOF received")
			return true, "EOF"
		}
		if err != nil {
			fmt.Println("Error receiving message from server:", err)
			return false, MSG_ERR
		}
		if n == 0 {
			// nothing read
			fmt.Println("Nothing read from server - EOF?")
			return true, "EOF"
		}

		msg := string(buf[:n]) // successfully read to buffer
		fmt.Println("SERVER:", msg)
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
		fmt.Println("Error receiving message from server")
		return false, MSG_ERR
	}

	if strings.TrimSpace(msg) != "" {
		prevMessage = msg // update the prevMessage
	}
	return res, msg
}

// the first server's QRCode will contain the srvAddress, so the client could read it and connect
func getInitMessageFromServer(c net.Conn, buf []byte) (bool, string) {
	utils.PrintDebugMessage("enter getInitMessageFromServer", DEBUG)
	defer utils.PrintDebugMessage("exit getInitMessageFromServer\n", DEBUG)

	utils.PrintDebugMessage("Trying to get the INIT message from server", DEBUG)

	// scan QRCode containing server address
	//res, msg := utils.DecodeMessageFile("qr_msg.png", DEBUG)
	//res, msg := utils.ScanMessage(WEBCAM, DEBUG)
	res, msg := receiveMessageFromServer(c, buf)
	if !res {
		fmt.Println("Error getting the INIT message from server")
		return false, MSG_ERR
	}

	utils.PrintDebugMessage("Extracted INIT message = "+msg, DEBUG)
	return true, msg
}

// create the destination file before usage
func createFile(dst string) bool {
	utils.PrintDebugMessage("enter createFile", DEBUG)
	defer utils.PrintDebugMessage("exit createFile\n", DEBUG)

	utils.PrintDebugMessage("Trying to get FileInfo", DEBUG)
	var _, err = os.Stat(dst)

	if os.IsNotExist(err) {
		utils.PrintDebugMessage("Trying to create file", DEBUG)
		destination, err := os.Create(dst)
		if err != nil {
			fmt.Println("Error creating file:", err)
			return false
		}
		defer destination.Close()
	} else {
		fmt.Println("File already exists - provide another one")
		return false
	}

	fmt.Println("File created successfully:", dst)
	return true
}

// append destination file with the new message
// no need to deal with EOF
func appendFile(dst, message string) bool {
	utils.PrintDebugMessage("enter appendFile", DEBUG)
	defer utils.PrintDebugMessage("exit appendFile\n", DEBUG)

	if len(message) == 0 {
		fmt.Println("Empty message - ignore")
		return true
	}

	utils.PrintDebugMessage("Trying to open file", DEBUG)
	// os.O_APPEND flag - write at the end of the file
	// os.O_CREATE flag - create the file if it does not exist
	destination, err := os.OpenFile(dst, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		fmt.Println("Error openning file:", err)
		return false
	}
	defer destination.Close()

	utils.PrintDebugMessage("Trying to append file", DEBUG)
	n, err := fmt.Fprint(destination, message)
	if err != nil {
		fmt.Println("Error appending file:", err)
		return false
	}
	if n == 0 {
		fmt.Println("Nothing was written")
		return false
	}

	// some bytes were written
	utils.PrintDebugMessage("File appended", DEBUG)
	return true
}

// core function for processing the connection with server
// 2022/02/19 - Diana. No need to pass the server address, this will be scanned from QR Code
//func processConnection(srvAddress, source, destination string) {
func processConnection(source, destination string) {
	utils.PrintDebugMessage("enter processConnection", DEBUG)
	defer utils.PrintDebugMessage("exit processConnection\n", DEBUG)

	seqExpected = 0                 // sequence number of the expected message
	var c net.Conn = nil            // nesessary for TCP connection, for QR Codes just leave it nil
	buf := make([]byte, bufferSize) // allocate the buffer - useful for TCP connection

	// receive the initial message from server
	// the first server's QRCode will contain the srvAddress, so the client could read it and connect
	utils.PrintDebugMessage("Trying to receive INIT message from server", DEBUG)
	result, srvAddress := getInitMessageFromServer(c, buf)
	if !result {
		fmt.Println("Client failed to receive the INIT message from server")
		return
	}
	utils.PrintDebugMessage("Server address received: "+srvAddress, DEBUG)

	// it's time to forget about TCP...
	/*
		// this is a connection request to the server
		utils.PrintDebugMessage("Trying to dial server", DEBUG)
		c, err := net.Dial("tcp", srvAddress)
		if err != nil {
			fmt.Println("Client failed to dial server:", err)
			fmt.Println("Client is exiting with error...")
			return
		}

		fmt.Println("Connection with server established")
		defer c.Close() // don't forget to close the connection
	*/

	defer fmt.Println("Client is exiting...")

	// connection established
	// client will send short messages in line format, but receive buffered bounded messages
	// send the initial request having source filename and the buffer-size
	utils.PrintDebugMessage("Trying to send INIT message to server", DEBUG)
	if !sendMessageToServer(c, ""+source+","+strconv.Itoa(bufferSize)+MSG_SUFFIX) {
		fmt.Println("Client failed to send INIT message to server")
		return
	}

	// main loop - eventually it will return
	for {
		// receive message from server
		res, msg := receiveMessageFromServer(c, buf)
		if !res {
			fmt.Println("Client failed to receive message from server")
			sendMessageToServer(c, "ERR"+MSG_SUFFIX) // send error message to server to close connection
			return
		}
		if strings.TrimSpace(msg) == "EOF" {
			// server finished the transfer
			fmt.Println("Client received EOF message from server (server finished successfully)")
			return
		}
		if strings.TrimSpace(msg) == "FIN" {
			// server finished and closed the connection, exit too
			fmt.Println("Client received FIN message from server (server finished successfully)")
			return
		}
		if strings.TrimSpace(msg) == "ERR" {
			// server error and closed the connection, exit too
			fmt.Println("Client received ERR message from server (server encountered an error)")
			return
		}
		if strings.TrimSpace(msg) == "" {
			continue
		}
		if int(msg[0]) != utils.DIGIT_OFFSET && int(msg[0]) != utils.DIGIT_OFFSET+1 {
			// this isn't the message expected, maybe old QR Code is scanned
			continue
		}

		// message received:
		message := ""
		seq := int(msg[0]) - utils.DIGIT_OFFSET // first byte is for sequence number
		if len(msg) > 1 {
			message = msg[1:] // message itself starts from second byte
		}

		// if arrived seq differs, then just try to receive another one (done by the loop)
		// otherwise, this is expected message -> proceed
		if seq == seqExpected {
			// append the message to destination file
			if !appendFile(destination, message) {
				fmt.Println("Client failed to process message")
				sendMessageToServer(c, "ERR"+MSG_SUFFIX) // send error message to server to close conection
				return
			}

			// increment sequence number to proceed
			seqExpected = utils.IncrementSEQ(seqExpected)

			// send ACK only for success
			ackSeq = 1 - seqExpected // tell which frame is being acked
			// send ack to server, so it can send next message
			ack := strconv.Itoa(ackSeq) + MSG_ACK + MSG_SUFFIX
			sendMessageToServer(c, ack)

		}
		// re-transmit ACK
		/*
			ackSeq = 1 - seqExpected // tell which frame is being acked
			// send ack to server, so it can send next message
			ack := strconv.Itoa(ackSeq) + MSG_ACK + MSG_SUFFIX
			sendMessageToServer(c, ack)
		*/
	}
}

func main() {
	utils.PrintDebugMessage("enter main", DEBUG)
	defer utils.PrintDebugMessage("exit main\n", DEBUG)

	utils.PrintDebugMessage("Checking command line arguments", DEBUG)
	args := os.Args
	// 2022/02/19 - Diana. No need to pass the server address, this will be scanned from QR Code
	// if len(args) != 5 {
	// 	fmt.Printf("Usage: %s SERVER:PORT source destination BUFFER_SIZE\n", filepath.Base(args[0]))
	// 	return
	// }
	if len(args) != 5 {
		fmt.Printf("Usage: %s source destination BUFFER_SIZE WEBCAM\n", filepath.Base(args[0]))
		return
	}

	// handle arguments passed

	// 2022/02/19 - Diana. No need to pass the server address, this will be scanned from QR Code
	// srvAddress := args[1]
	// source := args[2]
	// destination := args[3]
	// bufSize, err := strconv.Atoi(args[4])
	source := args[1]
	destination := args[2]
	bufSize, err := strconv.Atoi(args[3])
	webcamPath = args[4]

	if err != nil {
		fmt.Println("Bad parameter - buffer-size must be integer")
		fmt.Println("Error converting string to integer:", err)
		return
	}
	// validate the buffer-size
	if bufSize < MIN_BUF_SIZE || bufSize > MAX_BUF_SIZE {
		fmt.Println("Bad parameter - buffer-size must be in range of", MIN_BUF_SIZE, MAX_BUF_SIZE)
		return
	}
	bufferSize = bufSize
	// validate the destination file
	if !createFile(destination) {
		fmt.Println("Resolve the problem and run the client program again...")
		return
	}
	utils.PrintDebugMessage("Command line arguments - OK", DEBUG)

	// client is good to start
	fmt.Println("Client is running!")

	utils.PrintDebugMessage("Trying to process connection with server", DEBUG)
	// 2022/02/19 - Diana. No need to pass the server address, this will be scanned from QR Code
	//processConnection(srvAddress, source, destination)
	processConnection(source, destination)
}
