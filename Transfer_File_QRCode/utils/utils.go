package utils

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	webcam "github.com/blackjack/webcam"
	goqr "github.com/liyue201/goqr"
	qrcode "github.com/skip2/go-qrcode"
)

const MAX_SEQ int = 1
const DIGIT_OFFSET int = 48 // ASCII 48 is 0
const RETRY int = 10
const WAIT_TIME_SEND time.Duration = 2000 * time.Millisecond
const WAIT_TIME_RECEIVE time.Duration = 100 * time.Millisecond
const WAIT_TIME_FRAME uint32 = 3 // in seconds
const MSG_ERR string = "ERR"
const MSG_CLIENT string = "CLIENT"
const MSG_SERVER string = "SERVER"

// deal with some jpeg format error
var (
	dhtMarker = []byte{255, 196}
	dht       = []byte{1, 162, 0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 0, 3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 125, 1, 2, 3, 0, 4, 17, 5, 18, 33, 49, 65, 6, 19, 81, 97, 7, 34, 113, 20, 50, 129, 145, 161, 8, 35, 66, 177, 193, 21, 82, 209, 240, 36, 51, 98, 114, 130, 9, 10, 22, 23, 24, 25, 26, 37, 38, 39, 40, 41, 42, 52, 53, 54, 55, 56, 57, 58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100, 101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 131, 132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150, 151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182, 183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213, 214, 215, 216, 217, 218, 225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 17, 0, 2, 1, 2, 4, 4, 3, 4, 7, 5, 4, 4, 0, 1, 2, 119, 0, 1, 2, 3, 17, 4, 5, 33, 49, 6, 18, 65, 81, 7, 97, 113, 19, 34, 50, 129, 8, 20, 66, 145, 161, 177, 193, 9, 35, 51, 82, 240, 21, 98, 114, 209, 10, 22, 36, 52, 225, 37, 241, 23, 24, 25, 26, 38, 39, 40, 41, 42, 53, 54, 55, 56, 57, 58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100, 101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 130, 131, 132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150, 151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182, 183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213, 214, 215, 216, 217, 218, 226, 227, 228, 229, 230, 231, 232, 233, 234, 242, 243, 244, 245, 246, 247, 248, 249, 250}
	sosMarker = []byte{255, 218}
)

// for webcam usage
type FrameSizes []webcam.FrameSize

// additional printings when tracing the flow
func PrintDebugMessage(msg string, DEBUG bool) {
	if DEBUG {
		fmt.Println("[DEBUG]:", msg)
	}
}

// increment seq circularly
func IncrementSEQ(seq int) int {
	if seq < MAX_SEQ {
		return seq + 1
	}
	return 0
}

// deal with some jpeg format error
func addMotionDht(frame []byte) []byte {
	jpegParts := bytes.Split(frame, sosMarker)
	return append(jpegParts[0], append(dhtMarker, append(dht, append(sosMarker, jpegParts[1]...)...)...)...)
}

// encode the string message into QR Code
func encodeMessage(message string, DEBUG bool) bool {
	PrintDebugMessage("enter encodeMessage", DEBUG)
	defer PrintDebugMessage("exit encodeMessage\n", DEBUG)

	// write the QR Code to file - not needed anymore
	//imgFile := "qr_msg.png"
	//err := qrcode.WriteFile(message, qrcode.Medium, 256, imgFile)

	//q, err := qrcode.New(message, qrcode.Medium)
	q, err := qrcode.New(message, qrcode.Highest)
	if err != nil {
		fmt.Println("Error creating qrcode:", err)
		return false
	}

	fmt.Print(q.ToSmallString(true)) // just print the qrcode to terminal!!!
	//fmt.Print(q.ToString(false)) // just print the qrcode to terminal!!!
	return true
}

// decodes QR Code from image data into string message
func decodeMessage(imgData []byte, DEBUG bool) (bool, string) {
	PrintDebugMessage("enter decodeMessage", DEBUG)
	defer PrintDebugMessage("exit decodeMessage\n", DEBUG)

	PrintDebugMessage("Trying to decode image", DEBUG)

	// there is some JPEG format error - fix it
	//r := bytes.NewReader(imgData)
	r := bytes.NewReader(addMotionDht(imgData))

	img, _, err := image.Decode(r)
	if err != nil {
		fmt.Println("Error decoding image:", err)
		return false, MSG_ERR
	}

	PrintDebugMessage("Trying to recognize QR Code from image", DEBUG)
	qrCodes, err := goqr.Recognize(img)
	if err != nil {
		if err == goqr.ErrNoQRCode {
			//return true, "\n"
			return true, ""
		} else {
			fmt.Println("Error recognizing QR Code image:", err)
			return false, MSG_ERR
		}
	}

	PrintDebugMessage("Trying to walk through recognized QR Codes", DEBUG)
	for _, qrCode := range qrCodes {
		message := string(qrCode.Payload)
		// return the first message - we expect only one
		return true, message
	}

	// didn't return successful message, so return badly
	return false, MSG_ERR
}

// decodes QR Code from image file into string message
func decodeMessageFile(path string, DEBUG bool) (bool, string) {
	PrintDebugMessage("enter decodeMessageFile", DEBUG)
	defer PrintDebugMessage("exit decodeMessageFile\n", DEBUG)

	PrintDebugMessage("Trying to read file "+path, DEBUG)
	imgData, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return false, MSG_ERR
	}

	// just call another function dealing with the image data
	return decodeMessage(imgData, DEBUG)
}

// not needed anymore - just use in-memory, without saving to file
// save the single frame into PNG file
func saveFrameToImage(path string, imgData []byte, DEBUG bool) bool {
	PrintDebugMessage("enter saveFrameToImage", DEBUG)
	defer PrintDebugMessage("exit saveFrameToImage\n", DEBUG)

	PrintDebugMessage("Trying to decode image", DEBUG)

	// there is some JPEG format error - fix it
	//r := bytes.NewReader(imgData)
	r := bytes.NewReader(addMotionDht(imgData))

	img, _, err := image.Decode(r)
	if err != nil {
		fmt.Println("Error decoding image:", err)
		return false
	}

	PrintDebugMessage("Trying to create file", DEBUG)
	out, err := os.Create(path)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return false
	}
	defer out.Close()

	PrintDebugMessage("Trying to encode png", DEBUG)
	err = png.Encode(out, img)
	if err != nil {
		fmt.Println("Error encoding png:", err)
		return false
	}

	PrintDebugMessage("Frame saved to png file", DEBUG)
	return true
}

// set the webcam
func SetupWebcam(cam *webcam.Webcam, DEBUG bool) bool {
	PrintDebugMessage("Preparing the webcam", DEBUG)
	PrintDebugMessage("Trying to open webcam", DEBUG)

	PrintDebugMessage("Trying to get supported formats", DEBUG)
	formatDesc := cam.GetSupportedFormats()
	var formats []webcam.PixelFormat
	for f := range formatDesc {
		formats = append(formats, f)
	}
	if len(formats) == 0 {
		fmt.Println("No formats available")
		return false
	}
	if DEBUG {
		fmt.Println("Available formats: ")
		for i, value := range formats {
			fmt.Println(i+1, formatDesc[value])
		}
	}
	PrintDebugMessage("Set the first format available", DEBUG)
	format := formats[0]

	PrintDebugMessage("Trying to get supported frame sizes for format", DEBUG)
	frames := FrameSizes(cam.GetSupportedFrameSizes(format))
	if len(frames) == 0 {
		fmt.Println("No frame sizes for format available")
		return false
	}
	if DEBUG {
		fmt.Println("Supported frame sizes for format", formatDesc[format])
		for i, value := range frames {
			fmt.Println(i+1, value.GetString())
		}
	}
	//PrintDebugMessage("Set the last frame size available", DEBUG)
	//size := frames[len(frames)-1] // set the last frame size available
	PrintDebugMessage("Set the first frame size available", DEBUG)
	size := frames[0] // set the first frame size available

	PrintDebugMessage("Trying to set image format", DEBUG)
	f, w, h, err := cam.SetImageFormat(format, size.MaxWidth, size.MaxHeight)
	if err != nil {
		fmt.Println("Error setting image format:", err)
		return false
	}

	if DEBUG {
		fmt.Println("Resulting image format:", formatDesc[f], w, h)
	}
	PrintDebugMessage("Preparing the webcam - OK", DEBUG)

	PrintDebugMessage("Trying to start streaming", DEBUG)
	err = cam.StartStreaming()
	if err != nil {
		fmt.Println("Error starting streaming:", err)
		return false
	}

	return true
}

// scan the message using webcam
// dealing with webcam might be moved to higher level, for example into processing connection
// the reason everything is done here (and not in client/server implementation) is to keep client and server clean
// thet don't need to be aware of type of connection
func scanMessage(camPath string, DEBUG bool) (bool, string) {
	PrintDebugMessage("enter scanMessage", DEBUG)
	defer PrintDebugMessage("exit scanMessage\n", DEBUG)

	PrintDebugMessage("Preparing the webcam", DEBUG)
	PrintDebugMessage("Trying to open webcam", DEBUG)
	cam, err := webcam.Open(camPath) // open webcam
	if err != nil {
		fmt.Println("Error openning webcam:", err)
		return false, MSG_ERR
	}

	defer cam.Close() // don't forget to close the webcam

	PrintDebugMessage("Trying to get supported formats", DEBUG)
	formatDesc := cam.GetSupportedFormats()
	var formats []webcam.PixelFormat
	for f := range formatDesc {
		formats = append(formats, f)
	}
	if len(formats) == 0 {
		fmt.Println("No formats available")
		return false, MSG_ERR
	}
	if DEBUG {
		fmt.Println("Available formats: ")
		for i, value := range formats {
			fmt.Println(i+1, formatDesc[value])
		}
	}
	PrintDebugMessage("Set the first format available", DEBUG)
	format := formats[0]

	PrintDebugMessage("Trying to get supported frame sizes for format", DEBUG)
	frames := FrameSizes(cam.GetSupportedFrameSizes(format))
	if len(frames) == 0 {
		fmt.Println("No frame sizes for format available")
		return false, MSG_ERR
	}
	if DEBUG {
		fmt.Println("Supported frame sizes for format", formatDesc[format])
		for i, value := range frames {
			fmt.Println(i+1, value.GetString())
		}
	}
	//PrintDebugMessage("Set the last frame size available", DEBUG)
	//size := frames[len(frames)-1] // set the last frame size available
	PrintDebugMessage("Set the first frame size available", DEBUG)
	size := frames[0] // set the first frame size available

	PrintDebugMessage("Trying to set image format", DEBUG)
	f, w, h, err := cam.SetImageFormat(format, size.MaxWidth, size.MaxHeight)
	if err != nil {
		fmt.Println("Error setting image format:", err)
		return false, MSG_ERR
	}

	if DEBUG {
		fmt.Println("Resulting image format:", formatDesc[f], w, h)
	}
	PrintDebugMessage("Preparing the webcam - OK", DEBUG)

	PrintDebugMessage("Trying to start streaming", DEBUG)
	err = cam.StartStreaming()
	if err != nil {
		fmt.Println("Error starting streaming:", err)
		return false, MSG_ERR
	}

	// this is the scanning process...
	// no need in loop - just try to get the first frame
	//for retryIndex := 0; retryIndex < RETRY; retryIndex++ {
	PrintDebugMessage("Trying to wait for frame", DEBUG)
	err = cam.WaitForFrame(WAIT_TIME_FRAME)

	switch err.(type) {
	case nil:
	case *webcam.Timeout:
		fmt.Println("Error waiting for frame TIMEOUT:", err)
		//continue // when using the loop...
		return false, MSG_ERR // when using without loop
	default:
		fmt.Println("Error waiting for frame:", err)
		return false, MSG_ERR
	}

	PrintDebugMessage("Trying to read frame", DEBUG)
	fr, err := cam.ReadFrame()
	if err != nil {
		fmt.Println("Error reading frame:", err)
		return false, MSG_ERR
	}
	if len(fr) != 0 {
		// process the frame here

		// when using the file
		//saveFrameToImage("img.png", fr, DEBUG)
		//return decodeMessageFile("img.png", DEBUG)

		// just use in-memory
		res, msg := decodeMessage(fr, DEBUG)
		/*
			if res && msg == "" {
				continue // take another try
			}
		*/
		return res, msg
	}
	//}

	return false, MSG_ERR
}

// encode message into QRCode and display it in terminal
func SendMessage(c net.Conn, message string, from string, to string, DEBUG bool) bool {
	PrintDebugMessage("enter SendMessage", DEBUG)
	defer PrintDebugMessage("exit SendMessage\n", DEBUG)

	fmt.Println("[MSG]:", message, "[", time.Now(), "]")

	// use the QR Code...
	PrintDebugMessage("Trying to encode message into QR code", DEBUG)
	if !encodeMessage(message, DEBUG) {
		return false
	}

	// now it's time to forget about TCP...
	/*
		// use the TCP connection
		// disconnect server from TCP
		if from == MSG_CLIENT {
			PrintDebugMessage("Trying to send message to "+to+" = "+message, DEBUG)
			n, err := fmt.Fprint(c, message)
			if err != nil {
				fmt.Println("Error sending message to "+to+":", err)
				return false
			}
			if n == 0 {
				fmt.Println("Nothing was sent to", to)
				return false
			}
			// some bytes were sent
			fmt.Println(from+":", message, "("+strconv.Itoa(n)+" bytes)")
		}
	*/

	return true
}

// receive the message via QR Code
func ReceiveMessage(from string, to string, prevMessage string, camPath string, DEBUG bool) (bool, string) {
	PrintDebugMessage("enter ReceiveMessage", DEBUG)
	defer PrintDebugMessage("exit ReceiveMessage\n", DEBUG)

	PrintDebugMessage("Trying to receive message from "+from, DEBUG)
	res, msg := scanMessage(camPath, DEBUG)
	if !res {
		fmt.Println("Error scanning the message")
		return false, MSG_ERR
	}

	// don't print empty messages or duplicates
	if strings.TrimSpace(msg) != "" { //&& msg != prevMessage {
		fmt.Println(from+":", msg, "[", time.Now(), "]")
	}
	return true, msg
}

// Special function in Go language.
// The init function is used to initialize the state of a package.
// Go will automatically call the init function defined here,
// prior to calling the main function of a command line Go program.
func init() {
}
