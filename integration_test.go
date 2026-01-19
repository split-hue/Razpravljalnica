//go:build integration

package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"razpravljalnica/client"
)

const (
	headAddr = "localhost:9876"
	intAddr  = "localhost:9877"
	tailAddr = "localhost:9878"
	allNodes = "localhost:9876,localhost:9877,localhost:9878"
)

//
// BASIC TEST
//
// Zalaufamo head, intermediate, tail
// Kreiramo
// - topic
// - message
// - like

func TestBasicFlow_CreateTopic_Post_Like(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	procs := startCluster(t)
	defer stopCluster(t, procs)

	// pocakamo da se porti zalaufajo
	waitTCP(t, headAddr, 5*time.Second)
	waitTCP(t, tailAddr, 5*time.Second)

	//
	// CLIENT
	//
	cl, err := client.NewClient(headAddr, tailAddr)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer cl.Close()

	// create user
	cl.CreateUser("test-user")
	if cl.GetCurrentUser() == nil {
		t.Fatalf("expected current user to be set")
	}

	// create topic - NA HEAD
	topic := cl.CreateTopic("test-topic")
	if topic == nil || topic.Id == 0 {
		t.Fatalf("CreateTopic error")
	}

	// pocakaj replicate - ali je topic visible on TAIL
	eventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		resp := cl.ListTopics()
		if resp == nil {
			return false
		}
		for _, tp := range resp.Topics {
			if tp.Id == topic.Id {
				return true
			}
		}
		return false
	}, "topic should appear on TAIL")

	// post message - NA HEAD
	msg := cl.PostMessageToTopic(topic.Id, "TEST post message")
	if msg == nil || msg.Id == 0 {
		t.Fatalf("PostMessageToTopic error")
	}

	// pocakaj na replicate
	eventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		resp := cl.GetMessagesByTopic(topic.Id, 0, 0)
		if resp == nil {
			return false
		}
		for _, m := range resp.Messages {
			if m.Id == msg.Id && m.Text == "TEST post message" {
				return true
			}
		}
		return false
	}, "message should appear on TAIL")

	// like - NA HEAD
	cl.LikeMessage(topic.Id, msg.Id)

	// pocakaj reploiacte
	eventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		resp := cl.GetMessagesByTopic(topic.Id, 0, 0)
		if resp == nil {
			return false
		}
		for _, m := range resp.Messages {
			if m.Id == msg.Id && m.Likes >= 1 {
				return true
			}
		}
		return false
	}, "like should be reflected on TAIL")
}

type proc struct {
	name string
	cmd  *exec.Cmd
	out  *bytes.Buffer
}

func startCluster(t *testing.T) []proc {
	t.Helper()

	// ZAŽENEMO UKAZE KOT BI JIH MI
	// go run *.go -role head -p 9876 -successor localhost:9877 -all localhost:9876,localhost:9877,localhost:9878
	// go run *.go -role intermediate -p 9877 -successor localhost:9878 -all localhost:9876,localhost:9877,localhost:9878
	// go run *.go -role tail -p 9878 -all localhost:9876,localhost:9877,localhost:9878

	head := startNode(t, "head", "9876", intAddr, true)
	inter := startNode(t, "intermediate", "9877", tailAddr, true)
	tail := startNode(t, "tail", "9878", "", false)

	return []proc{head, inter, tail}
}

func stopCluster(t *testing.T, procs []proc) {
	t.Helper()

	for _, p := range procs {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
}

func startNode(t *testing.T, role, port, successor string, hasSuccessor bool) proc {
	t.Helper()

	args := []string{"run", ".", "-role", role, "-p", port, "-all", allNodes}
	if hasSuccessor {
		args = append(args, "-successor", successor)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd = exec.Command("go", args...)
	cmd.Dir = mustWD(t)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v\n%s", role, err, buf.String())
	}

	return proc{name: role, cmd: cmd, out: &buf}
}

func mustWD(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return wd
}

func waitTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server not reachable at %s within %s (os=%s)", addr, timeout, runtime.GOOS)
}

func eventually(t *testing.T, timeout, tick time.Duration, fn func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("timeout after %s: %s", timeout, msg)
}
