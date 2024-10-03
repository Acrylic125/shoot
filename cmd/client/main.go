package main

import (
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:2222")
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	// conn.Write([]byte{
	// 	0, 0, 0, 0, 0, 1,
	// })
	for {

	}
	defer conn.Close()
}
