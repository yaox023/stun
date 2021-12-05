package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
)

func main() {
	conn, err := net.ListenPacket("udp", ":3478")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for {
		buf := make([]byte, 1024)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			continue
		}
		go serve(conn, addr, buf[:n])
	}
}

func formHeader(msgLength int, transactionID *[]byte) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)

	buf.Write([]byte{0b00000001, 0b00000001})

	err := binary.Write(buf, binary.BigEndian, uint16(msgLength))
	if err != nil {
		return buf, err
	}

	err = binary.Write(buf, binary.BigEndian, uint32(0x2112A442))
	if err != nil {
		return buf, err
	}

	_, err = buf.Write(*transactionID)
	if err != nil {
		return buf, err
	}

	return buf, nil
}

// form a MAPPED-ADDRESS binding response
// 8 bits of value 0 + 8-bit address family + 16-bit port + fixed-length value representing the IP address.
// If the address family is IPv6, the address MUST be 128 bits.
func formAddr(addr *net.UDPAddr) (*bytes.Buffer, error) {
	var err error
	buf := new(bytes.Buffer)

	err = buf.WriteByte(0)
	if err != nil {
		return buf, err
	}

	ipv4 := addr.IP.To4()
	if ipv4 != nil {
		err = buf.WriteByte(1)
		if err != nil {
			return buf, err
		}
		err = binary.Write(buf, binary.BigEndian, uint16(addr.Port))
		if err != nil {
			return buf, err
		}
		_, err = buf.Write(ipv4)
		if err != nil {
			return buf, err
		}
		return buf, nil
	}

	ipv6 := addr.IP.To16()
	if ipv6 != nil {
		err = buf.WriteByte(2)
		if err != nil {
			return buf, err
		}
		err := binary.Write(buf, binary.BigEndian, uint16(addr.Port))
		if err != nil {
			return buf, err
		}
		_, err = buf.Write(ipv6)
		if err != nil {
			return buf, err
		}
		return buf, nil
	}

	return buf, errors.New("Invalid ip addr: " + addr.String())
}

func formAttr(addr *net.UDPAddr) (*bytes.Buffer, error) {
	var err error
	buf := new(bytes.Buffer)

	// attr type
	err = binary.Write(buf, binary.BigEndian, uint16(1))
	if err != nil {
		return buf, nil
	}

	addrBuf, err := formAddr(addr)
	if err != nil {
		return buf, err
	}
	addrBytes := addrBuf.Bytes()

	err = binary.Write(buf, binary.BigEndian, uint16(len(addrBytes)))
	if err != nil {
		return buf, err
	}
	buf.Write(addrBytes)

	// TODO 对齐
	return buf, nil
}

func formResponse(addr *net.UDPAddr, transactionID *[]byte) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)

	attrBuf, err := formAttr(addr)
	if err != nil {
		return buf, err
	}
	attrBytes := attrBuf.Bytes()

	headerBuf, err := formHeader(len(attrBytes), transactionID)
	if err != nil {
		return buf, nil
	}

	_, err = buf.Write(headerBuf.Bytes())
	if err != nil {
		return buf, err
	}

	_, err = buf.Write(attrBytes)
	if err != nil {
		return buf, err
	}

	return buf, nil
}

func serve(pc net.PacketConn, addr net.Addr, buf []byte) {
	fmt.Println("addr", addr)

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		fmt.Println("invalid remote addr: cannot convert to udp addr")
		return
	}

	// first two bit should be 0
	if (buf[0]>>6)&1 != 0 || (buf[0]>>7)&1 != 0 {
		fmt.Printf("invalid message: %b\n", buf[0])
		return
	}

	// only support class request
	// class request: 0b00
	c1 := buf[0] & 0
	c0 := (buf[1] >> 3) & 0
	if (c1 != 0) || c0 != 0 {
		fmt.Printf("class not supported: c1:%d, c0:%d\n", c1, c0)
		return
	}

	// only support method Binding
	msgType := binary.BigEndian.Uint16(buf[0:2])
	if msgType != 1 {
		fmt.Printf("method not supported: %b", msgType)
		return
	}

	// only deal with 0 attributes now
	msgLength := binary.BigEndian.Uint16(buf[2:4])
	if msgLength != 0 {
		fmt.Println("msgLength not supported", msgLength)
		return
	}

	magicCookie := binary.BigEndian.Uint32(buf[4:8])
	if magicCookie != 0x2112A442 {
		fmt.Printf("invalid magicCookie: %x\n", magicCookie)
		return
	}

	transactionID := buf[8:]
	fmt.Println("transactionID", transactionID)

	resBuf, err := formResponse(udpAddr, &transactionID)
	if err != nil {
		fmt.Println("form response error", err.Error())
		return
	}

	pc.WriteTo(resBuf.Bytes(), addr)
}
