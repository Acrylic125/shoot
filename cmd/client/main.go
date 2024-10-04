package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:2222")
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer conn.Close()
	for {
		if _, err := conn.Write([]byte{
			0, 0, 0,
		}); err != nil {
			fmt.Println("Error writing:", err)
			return
		}
		if _, err := conn.Write([]byte{
			0, 1, 12,
			0xf, 0, 0, 1,
			0xff, 0xff, 0xff, 0,
			0, 0, 0, 0xff,
		}); err != nil {
			fmt.Println("Error writing:", err)
			return
		}
		time.Sleep(1 * time.Second)
	}
}
