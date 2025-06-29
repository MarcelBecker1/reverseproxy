/* 	Tcp message framing with length prefixing
	4 Byte big-endian

Motivation:
	- TCP as streaming protocol
	- We Read from stream and write to stream
		- If message is larger it gets split over multiple packets
		- Multiple smaller messages can be combined with one Read
	-> Especially an issue when multiple messages arive
	-> Have to know when a message is complete and where the next message starts

Packets:
	4 bytes length
	N bytes data

Is length data + header or only data length?
Do we need larger header with checksums or something? Checksums is handled by TCP itself i think
	Should be covered, I only need it for knowing where to split the messages
*/

package netutils

import (
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"strings"
)

func SendMessage(conn io.Writer, msg string, log *slog.Logger) error {
	buffer := []byte(msg)
	length := len(buffer)
	if length > 4294967295 {
		// We should split it
		log.Error("message is too large for 4bytes", "length", length)
		return errors.New("invalid input")
	}

	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(length))

	message := append(lengthBytes, buffer...)

	n, err := conn.Write(message)
	if err != nil {
		log.Error("error sending data", "error", err)
		return err
	}

	log.Info("sent data", "bytes", n, "message", msg)
	return nil
}

func ReadMessage(conn io.Reader, log *slog.Logger) (string, uint32, error) {
	length := make([]byte, 4)
	if _, err := io.ReadFull(conn, length); err != nil {
		if err != io.EOF && !isConnectionClosed(err) {
			log.Error("failed to read length prefix", "error", err)
		}
		return "", 0, err
	}

	dataLength := binary.BigEndian.Uint32(length)
	data := make([]byte, dataLength)

	_, err := io.ReadFull(conn, data)
	if err != nil {
		if err != io.EOF && !isConnectionClosed(err) {
			log.Error("failed to read data", "error", err)
		}
		return "", 0, err
	}

	return string(data), dataLength, nil
}

func isConnectionClosed(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
