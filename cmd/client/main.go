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
	for {
		if _, err := conn.Write([]byte{
			0, 0, 0,
		}); err != nil {
			fmt.Println("Error writing:", err)
			return
		}
		time.Sleep(1 * time.Second)
	}
	defer conn.Close()
}
