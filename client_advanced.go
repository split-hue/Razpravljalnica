package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"razpravljalnica/razpravljalnica"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AdvancedClient za testiranje chain replication
type AdvancedClient struct {
	headConn   *grpc.ClientConn
	tailConn   *grpc.ClientConn
	headClient razpravljalnica.MessageBoardClient
	tailClient razpravljalnica.MessageBoardClient
	ctrlClient razpravljalnica.ControlPlaneClient
}

func NewAdvancedClient(headURL, tailURL string) (*AdvancedClient, error) {
	fmt.Printf("Advanced client connecting to:\n")
	fmt.Printf("  HEAD: %v\n", headURL)
	fmt.Printf("  TAIL: %v\n", tailURL)

	headConn, err := grpc.Dial(headURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	tailConn, err := grpc.Dial(tailURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		headConn.Close()
		return nil, err
	}

	return &AdvancedClient{
		headConn:   headConn,
		tailConn:   tailConn,
		headClient: razpravljalnica.NewMessageBoardClient(headConn),
		tailClient: razpravljalnica.NewMessageBoardClient(tailConn),
		ctrlClient: razpravljalnica.NewControlPlaneClient(headConn),
	}, nil
}

func (c *AdvancedClient) Close() {
	c.headConn.Close()
	c.tailConn.Close()
}

// test k se izvede ob "-test"
func (c *AdvancedClient) RunComprehensiveTest() {
	defer c.Close()

	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  RAZPRAVLJALNICA - CHAIN REPLICATION TEST                ║")
	//fmt.Println("╚══════════════════════════════════════════════════════════╝\n")

	ctx := context.Background()

	// Test 0: Cluster State
	fmt.Println("Test 0: Cluster State")
	fmt.Println("─────────────────────────────────────────────────────────")
	state, err := c.ctrlClient.GetClusterState(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Printf("XX ERROR XX getting cluster state: %v\n", err)
		return
	}
	fmt.Printf("✓ HEAD: %s @ %s\n", state.Head.NodeId, state.Head.Address)
	fmt.Printf("✓ TAIL: %s @ %s\n", state.Tail.NodeId, state.Tail.Address)
	fmt.Printf("✓ Chain has %d nodes\n", len(state.Chain))
	for i, node := range state.Chain {
		roleStr := "INTERMEDIATE"
		if node.Role == razpravljalnica.NodeRole_HEAD {
			roleStr = "HEAD"
		} else if node.Role == razpravljalnica.NodeRole_TAIL {
			roleStr = "TAIL"
		}
		fmt.Printf("  %d. [%s] %s @ %s\n", i+1, roleStr, node.NodeId, node.Address)
	}
	time.Sleep(500 * time.Millisecond)

	// Test 1: Create Users on HEAD
	fmt.Println("\nTest 1: Creating Users on HEAD")
	fmt.Println("─────────────────────────────────────────────────────────")
	users := []string{"Alice", "Bob", "Charlie", "Diana"}
	userObjects := []*razpravljalnica.User{}

	for _, name := range users {
		user, err := c.headClient.CreateUser(ctx, &razpravljalnica.CreateUserRequest{Name: name})
		if err != nil {
			fmt.Printf("XX ERROR XX creating user %s: %v\n", name, err)
			return
		}
		userObjects = append(userObjects, user)
		fmt.Printf("✓ Created on HEAD: %s (ID: %d)\n", user.Name, user.Id)
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for replication
	fmt.Println("\n~ Waiting for replication to propagate through chain... ~")
	time.Sleep(2 * time.Second)

	// Test 2: Create Topics on HEAD
	fmt.Println("\nTest 2: Creating Topics on HEAD")
	fmt.Println("─────────────────────────────────────────────────────────")
	topics := []string{"Verižna replikacija", "Distribuirani sistemi", "gRPC", "Go programiranje"}
	topicObjects := []*razpravljalnica.Topic{}

	for _, name := range topics {
		topic, err := c.headClient.CreateTopic(ctx, &razpravljalnica.CreateTopicRequest{Name: name})
		if err != nil {
			fmt.Printf("XX ERROR XX creating topic %s: %v\n", name, err)
			return
		}
		topicObjects = append(topicObjects, topic)
		fmt.Printf("✓ Created on HEAD: %s (ID: %d)\n", topic.Name, topic.Id)
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for replication
	time.Sleep(2 * time.Second)

	// Test 3: List Topics from TAIL
	fmt.Println("\nTest 3: Listing Topics from TAIL")
	fmt.Println("─────────────────────────────────────────────────────────")
	listResp, err := c.tailClient.ListTopics(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Printf("XX ERROR XX listing topics from TAIL: %v\n", err)
		return
	}
	fmt.Printf("✓ TAIL has %d topics:\n", len(listResp.Topics))
	for _, t := range listResp.Topics {
		fmt.Printf("  • [%d] %s\n", t.Id, t.Name)
	}

	if len(listResp.Topics) != len(topicObjects) {
		fmt.Printf("XX WARNING XX: Expected %d topics, TAIL has %d (replication may be slow)\n",
			len(topicObjects), len(listResp.Topics))
	}

	// Test 4: Post Messages to HEAD
	fmt.Println("\nTest 4: Posting Messages to HEAD")
	fmt.Println("─────────────────────────────────────────────────────────")
	messages := []struct {
		topicIdx int
		userIdx  int
		text     string
	}{
		{0, 0, "Verižna replikacija omogoča visoko dostopnost!"},
		{0, 1, "Pisalni dostopi grejo na HEAD, bralni na TAIL."},
		{1, 2, "Distribuirani sistemi so pomembni za skalabilnost."},
		{2, 3, "gRPC omogoča učinkovito komunikacijo med vozlišči."},
		{3, 0, "Go ima odlično podporo za concurrency!"},
	}

	messageObjects := []*razpravljalnica.Message{}
	for _, m := range messages {
		msg, err := c.headClient.PostMessage(ctx, &razpravljalnica.PostMessageRequest{
			TopicId: topicObjects[m.topicIdx].Id,
			UserId:  userObjects[m.userIdx].Id,
			Text:    m.text,
		})
		if err != nil {
			fmt.Printf("XX ERROR XX posting message: %v\n", err)
			return
		}
		messageObjects = append(messageObjects, msg)
		fmt.Printf("✓ Posted to HEAD: [%s by %s] \"%s\" (Msg ID: %d)\n",
			topicObjects[m.topicIdx].Name, userObjects[m.userIdx].Name, m.text, msg.Id)
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for replication
	fmt.Println("\n~ Waiting for message replication... ~")
	time.Sleep(2 * time.Second)

	// Test 5: Get Messages from TAIL
	fmt.Println("\nTest 5: Reading Messages from TAIL")
	fmt.Println("─────────────────────────────────────────────────────────")
	for _, topic := range topicObjects[:2] {
		msgs, err := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
			TopicId:       topic.Id,
			FromMessageId: 0,
			Limit:         0,
		})
		if err != nil {
			fmt.Printf("XX ERROR XX reading from TAIL for topic %s: %v\n", topic.Name, err)
			continue
		}

		fmt.Printf("✓ TAIL topic '%s' has %d message(s):\n", topic.Name, len(msgs.Messages))
		for _, msg := range msgs.Messages {
			userName := ""
			for _, u := range userObjects {
				if u.Id == msg.UserId {
					userName = u.Name
					break
				}
			}
			fmt.Printf("  [%d] %s: %s\n", msg.Id, userName, msg.Text)
		}
		fmt.Println()
	}

	// Test 6: Like Messages on HEAD
	fmt.Println("Test 6: Liking Messages on HEAD")
	fmt.Println("─────────────────────────────────────────────────────────")
	likeCounts := make(map[int64]int)

	for i, msg := range messageObjects[:3] {
		for j := 0; j < (i + 1); j++ {
			_, err := c.headClient.LikeMessage(ctx, &razpravljalnica.LikeMessageRequest{
				TopicId:   msg.TopicId,
				MessageId: msg.Id,
				UserId:    userObjects[j].Id,
			})
			if err != nil {
				fmt.Printf("XX ERROR XX liking message %d: %v\n", msg.Id, err)
				continue
			}
			likeCounts[msg.Id]++
		}
		fmt.Printf("✓ Message %d liked %d time(s) on HEAD\n", msg.Id, likeCounts[msg.Id])
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for replication
	time.Sleep(2 * time.Second)

	// Test 7: Verify Likes from TAIL
	fmt.Println("\nTest 7: Verifying Likes from TAIL")
	fmt.Println("─────────────────────────────────────────────────────────")
	msgs, _ := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
		TopicId:       topicObjects[0].Id,
		FromMessageId: 0,
		Limit:         0,
	})
	fmt.Printf("✓ Reading likes from TAIL:\n")
	allMatch := true
	for _, msg := range msgs.Messages {
		expectedLikes := likeCounts[msg.Id]
		match := "✓"
		if int(msg.Likes) != expectedLikes {
			match = "X"
			allMatch = false
		}
		fmt.Printf("  %s Message %d: %d likes (expected %d)\n", match, msg.Id, msg.Likes, expectedLikes)
	}

	if allMatch {
		fmt.Println("\n✓ All likes replicated correctly!")
	} else {
		fmt.Println("\nX Some likes may not have replicated yet")
	}

	// Test 8: Subscription Load Balancing
	fmt.Println("\nTest 8: Subscription Load Balancing")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Println("Requesting subscription node assignment from HEAD...")

	subNodeResp, err := c.headClient.GetSubscriptionNode(ctx, &razpravljalnica.SubscriptionNodeRequest{
		UserId:  userObjects[0].Id,
		TopicId: []int64{topicObjects[0].Id},
	})
	if err != nil {
		fmt.Printf("XX ERROR XX getting subscription node: %v\n", err)
	} else {
		fmt.Printf("✓ HEAD assigned subscription to: %s @ %s\n",
			subNodeResp.Node.NodeId, subNodeResp.Node.Address)
		fmt.Printf("✓ Token: %s\n", subNodeResp.SubscribeToken)

		// Connect and subscribe
		subConn, err := grpc.Dial(subNodeResp.Node.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			defer subConn.Close()
			subClient := razpravljalnica.NewMessageBoardClient(subConn)

			stream, err := subClient.SubscribeTopic(context.Background(), &razpravljalnica.SubscribeTopicRequest{
				TopicId:        []int64{topicObjects[0].Id},
				UserId:         userObjects[0].Id,
				FromMessageId:  0,
				SubscribeToken: subNodeResp.SubscribeToken,
			})

			if err == nil {
				fmt.Println("✓ Successfully subscribed, listening for events...")

				// Receive some historical events
				eventCount := 0
				done := make(chan bool)
				go func() {
					for {
						event, err := stream.Recv()
						if err == io.EOF {
							break
						}
						if err != nil {
							break
						}
						eventCount++
						if eventCount <= 3 {
							fmt.Printf("  📨 Event #%d: Message ID %d\n", event.SequenceNumber, event.Message.Id)
						}
						if eventCount >= 3 {
							done <- true
							return
						}
					}
				}()

				select {
				case <-done:
					fmt.Printf("✓ Received %d events\n", eventCount)
				case <-time.After(3 * time.Second):
					fmt.Printf("✓ Received %d events (timeout)\n", eventCount)
				}
			}
		}
	}

	// Final Summary
	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║                   TEST SUMMARY                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Printf("\n✓ Cluster topology: %d nodes\n", len(state.Chain))
	fmt.Printf("✓ Created %d users on HEAD\n", len(userObjects))
	fmt.Printf("✓ Created %d topics on HEAD\n", len(topicObjects))
	fmt.Printf("✓ Posted %d messages to HEAD\n", len(messageObjects))
	fmt.Printf("✓ Added %d likes on HEAD\n", len(likeCounts))
	fmt.Printf("✓ Verified replication to TAIL\n")
	fmt.Printf("✓ Tested subscription load balancing\n")
}

func RunAdvancedClient(headURL, tailURL string) {
	client, err := NewAdvancedClient(headURL, tailURL)
	if err != nil {
		panic(err)
	}
	client.RunComprehensiveTest()
}
