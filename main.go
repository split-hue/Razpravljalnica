// Razpravljalnica - distribuirana spletna storitev z verižno replikacijo
//
// Zagon strežnikov (3 vozlišča primer):
//
// Terminal 1 - HEAD:
//     go run *.go -role head -p 9876 -successor localhost:9877 -all localhost:9876,localhost:9877,localhost:9878
//
// Terminal 2 - INTERMEDIATE:
//     go run *.go -role intermediate -p 9877 -successor localhost:9878 -all localhost:9876,localhost:9877,localhost:9878
//
// Terminal 3 - TAIL:
//     go run *.go -role tail -p 9878 -all localhost:9876,localhost:9877,localhost:9878
//
// Terminal 4 - Client:
//     go run *.go -head localhost:9876 -tail localhost:9878
//     go run *.go -head localhost:9876 -tail localhost:9878 -test

package main

import (
	"flag"
	"fmt"
	"strings"

	"razpravljalnica/razpravljalnica"
)

func main() {
	//server flags
	roleStr := flag.String("role", "", "node role: head, intermediate, or tail")
	port := flag.Int("p", 9876, "port number")
	successor := flag.String("successor", "", "successor node address (e.g., localhost:9877)")
	allNodes := flag.String("all", "", "comma-separated list of all node addresses for load balancing")

	//client flags
	headAddr := flag.String("head", "", "head node address for client")
	tailAddr := flag.String("tail", "", "tail node address for client")
	testMode := flag.Bool("test", false, "run comprehensive test")

	flag.Parse()

	//preveri način delovanja
	if *headAddr != "" && *tailAddr != "" {
		// client mode--------------------------
		if *testMode {
			RunAdvancedClient(*headAddr, *tailAddr)
		} else {
			ClientMain(*headAddr, *tailAddr)
		}
		return
	}
	if *roleStr == "" {
		fmt.Println("Error: must specify either:")
		fmt.Println("  Server mode: -role <head|intermediate|tail> -p <port> [-successor <addr>] -all <addr1,addr2,...>")
		fmt.Println("  Client mode: -head <addr> -tail <addr> [-test]")
		fmt.Println("\nExample 3-node setup:")
		fmt.Println("  Node 1 (HEAD):         go run . -role head -p 9876 -successor localhost:9877 -all localhost:9876,localhost:9877,localhost:9878")
		fmt.Println("  Node 2 (INTERMEDIATE): go run *.go -role intermediate -p 9877 -successor localhost:9878 -all localhost:9876,localhost:9877,localhost:9878")
		fmt.Println("  Node 3 (TAIL):         go run *.go -role tail -p 9878 -all localhost:9876,localhost:9877,localhost:9878")
		fmt.Println("  Client:                go run *.go -head localhost:9876 -tail localhost:9878")
		return
	}

	//server mode----------------------------------
	var role razpravljalnica.NodeRole //glej .proto
	switch strings.ToLower(*roleStr) {
	case "head":
		role = razpravljalnica.NodeRole_HEAD
	case "intermediate":
		role = razpravljalnica.NodeRole_INTERMEDIATE
	case "tail":
		role = razpravljalnica.NodeRole_TAIL
	default:
		fmt.Printf("Error: invalid role '%s'. Must be: head, intermediate, or tail\n", *roleStr)
		return
	}

	if *allNodes == "" {
		fmt.Println("Error: -all flag is required (comma-separated list of all node addresses)")
		return
	}

	allNodeAddrs := strings.Split(*allNodes, ",")
	for i := range allNodeAddrs {
		allNodeAddrs[i] = strings.TrimSpace(allNodeAddrs[i])
	}

	url := fmt.Sprintf(":%d", *port)
	Server(url, role, *successor, allNodeAddrs)
}
