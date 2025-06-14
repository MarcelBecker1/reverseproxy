/* 	Tcp packet framing

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
Do we need larger header with checksums or something?
	Should be covered, I only need it for knowing where to split the messages
*/

package framing
