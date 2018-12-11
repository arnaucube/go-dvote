package data

import (
	"os"
	"fmt"
	"bytes"
	"io/ioutil"
	shell "github.com/ipfs/go-ipfs-api"
)

type votePacket struct {
	PID uint
	Nullifier string
	Vote  string
	Franchise string
}

func publish(object []byte) (string) {
	sh := shell.NewShell("localhost:5001")
	cid, err := sh.Add(bytes.NewBuffer(object))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
	return cid
}

func pin(path string) {
	sh := shell.NewShell("localhost:5001")
	err := sh.Pin(path)
	if err != nil{
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
}


func retrieve(hash string) ([]byte){
	sh := shell.NewShell("localhost:5001")
	reader, err := sh.Cat(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
	return content
}