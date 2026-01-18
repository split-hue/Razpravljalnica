package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"razpravljalnica/razpravljalnica"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Client struct {
	headConn    *grpc.ClientConn
	tailConn    *grpc.ClientConn
	headClient  razpravljalnica.MessageBoardClient
	tailClient  razpravljalnica.MessageBoardClient
	ctrlClient  razpravljalnica.ControlPlaneClient
	currentUser *razpravljalnica.User
	reader      *bufio.Reader

	// Shranjevanje subscriptions na topic
	subscriptions map[string]*Subscription
	subMutex      sync.Mutex
}

type Subscription struct {
	Conn   *grpc.ClientConn                                  // hrani gRPC povezavo na vozlišče, ki je dodeljeno za subscription
	Client razpravljalnica.MessageBoardClient                // gRPC client za komunikacijo
	Stream razpravljalnica.MessageBoard_SubscribeTopicClient // stream, po katerem prejemamo dogodke
	Done   chan struct{}                                     // kanal, s katerim gorutino lahko ustavimo
}

func NewClient(headURL, tailURL string) (*Client, error) {
	fmt.Printf("╔════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  gRPC Client connecting to Chain Replication Cluster  ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")
	fmt.Printf("HEAD: %v\n", headURL)
	fmt.Printf("TAIL: %v\n", tailURL)
	fmt.Println()

	// Poveži se z HEAD za pisalne operacije
	headConn, err := grpc.Dial(headURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HEAD: %v", err)
	}

	// Poveži se z TAIL za bralne operacije
	tailConn, err := grpc.Dial(tailURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		headConn.Close()
		return nil, fmt.Errorf("failed to connect to TAIL: %v", err)
	}

	return &Client{
		headConn:   headConn,
		tailConn:   tailConn,
		headClient: razpravljalnica.NewMessageBoardClient(headConn),
		tailClient: razpravljalnica.NewMessageBoardClient(tailConn),
		ctrlClient: razpravljalnica.NewControlPlaneClient(headConn),
		reader:     bufio.NewReader(os.Stdin),
	}, nil
}

func (c *Client) Close() {
	c.headConn.Close()
	c.tailConn.Close()

	// Zapremo tudi vse subscriptions
	c.subMutex.Lock()
	for _, sub := range c.subscriptions {
		close(sub.Done)
		sub.Stream.CloseSend()
		sub.Conn.Close()
	}
	c.subscriptions = nil
	c.subMutex.Unlock()

}

func (c *Client) Run() {
	defer c.Close()

	fmt.Println("\n=== Razpravljalnica Client (Chain Replication) ===")
	fmt.Println("Commands:")
	fmt.Println("  1 - Create User (HEAD)")
	fmt.Println("  2 - Create Topic (HEAD)")
	fmt.Println("  3 - List Topics (TAIL)")
	fmt.Println("  4 - Post Message (HEAD)")
	fmt.Println("  5 - Get Messages (TAIL)")
	fmt.Println("  6 - Like Message (HEAD)")
	fmt.Println("  7 - Subscribe to Topics")
	fmt.Println("  8 - Get Cluster State")
	fmt.Println("  9 - Demo Mode")
	fmt.Println("  0 - Exit")
	fmt.Println()

	for {
		fmt.Print("Enter command: ")
		input, _ := c.reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			c.createUser()
		case "2":
			c.createTopic()
		case "3":
			c.listTopics()
		case "4":
			c.postMessage()
		case "5":
			c.getMessages()
		case "6":
			c.likeMessage()
		case "7":
			c.subscribeTopic()
		case "8":
			c.getClusterState()
		case "9":
			c.demoMode()
		case "0":
			fmt.Println("Exiting...")
			return
		default:
			fmt.Println("Invalid command")
		}
	}
}

func (c *Client) createUser() {
	fmt.Print("Enter username: ")
	name, _ := c.reader.ReadString('\n')
	name = strings.TrimSpace(name)

	c.CreateUser(name)
}

func (c *Client) CreateUser(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := c.headClient.CreateUser(ctx, &razpravljalnica.CreateUserRequest{
		Name: name,
	})
	if err != nil {
		//fmt.Printf("Error creating user: %v\n", err)
		return
	}

	c.currentUser = user
	//fmt.Printf("✓ User created on HEAD: %s (ID: %d)\n", user.Name, user.Id)
}

func (c *Client) GetCurrentUser() *razpravljalnica.User {
	if c.currentUser == nil {
		return nil
	}
	return c.currentUser
}

func (c *Client) createTopic() {
	if c.currentUser == nil {
		fmt.Println("Please create a user first (command 1)")
		return
	}

	fmt.Print("Enter topic name: ")
	name, _ := c.reader.ReadString('\n')
	name = strings.TrimSpace(name)

	c.CreateTopic(name)
}

func (c *Client) CreateTopic(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.headClient.CreateTopic(ctx, &razpravljalnica.CreateTopicRequest{
		Name: name,
	})
	if err != nil {
		//fmt.Printf("Error creating topic: %v\n", err)
		return
	}

	//fmt.Printf("✓ Topic created on HEAD: %s (ID: %d)\n", topic.Name, topic.Id)
}

func (c *Client) listTopics() {

	response := c.ListTopics()

	if len(response.Topics) == 0 {
		fmt.Println("No topics available")
		return
	}

	fmt.Println("\n✓ Available topics (from TAIL):")
	for _, topic := range response.Topics {
		fmt.Printf("  [%d] %s\n", topic.Id, topic.Name)
	}
}

func (c *Client) ListTopics() *razpravljalnica.ListTopicsResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := c.tailClient.ListTopics(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Printf("Error listing topics from TAIL: %v\n", err)
		return nil
	}

	return response
}

func (c *Client) postMessage() {
	if c.currentUser == nil {
		fmt.Println("Please create a user first (command 1)")
		return
	}

	fmt.Print("Enter topic ID: ")
	topicIDStr, _ := c.reader.ReadString('\n')
	topicIDStr = strings.TrimSpace(topicIDStr)
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid topic ID")
		return
	}

	fmt.Print("Enter message: ")
	text, _ := c.reader.ReadString('\n')
	text = strings.TrimSpace(text)

	c.PostMessageToTopic(topicID, text)
}

func (c *Client) PostMessageToTopic(topicID int64, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.headClient.PostMessage(ctx, &razpravljalnica.PostMessageRequest{
		TopicId: topicID,
		UserId:  c.currentUser.Id,
		Text:    text,
	})
	if err != nil {
		//fmt.Printf("Error posting message to HEAD: %v\n", err)
		return
	}

	//fmt.Printf("✓ Message posted to HEAD (ID: %d)\n", message.Id)
}

func (c *Client) getMessages() {
	fmt.Print("Enter topic ID: ")
	topicIDStr, _ := c.reader.ReadString('\n')
	topicIDStr = strings.TrimSpace(topicIDStr)
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid topic ID")
		return
	}

	fmt.Print("From message ID (0 for all): ")
	fromIDStr, _ := c.reader.ReadString('\n')
	fromIDStr = strings.TrimSpace(fromIDStr)
	fromID, _ := strconv.ParseInt(fromIDStr, 10, 64)

	fmt.Print("Limit (0 for all): ")
	limitStr, _ := c.reader.ReadString('\n')
	limitStr = strings.TrimSpace(limitStr)
	limit, _ := strconv.ParseInt(limitStr, 10, 32)

	c.writeMessagesToConsole(topicID, fromID, limit)
}

func (c *Client) writeMessagesToConsole(topicID int64, fromID int64, limit int64) {

	response := c.GetMessagesByTopic(topicID, fromID, limit)

	if len(response.Messages) == 0 {
		fmt.Println("No messages found")
		return
	}

	fmt.Println("\n✓ Messages (from TAIL):")
	for _, msg := range response.Messages {
		fmt.Printf("  [%d] User %d: %s (Likes: %d)\n", msg.Id, msg.UserId, msg.Text, msg.Likes)
	}
}

func (c *Client) GetMessagesByTopic(topicID int64, fromID int64, limit int64) *razpravljalnica.GetMessagesResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
		TopicId:       topicID,
		FromMessageId: fromID,
		Limit:         int32(limit),
	})
	if err != nil {
		fmt.Printf("Error getting messages from TAIL: %v\n", err)
		return nil
	}

	return response
}

func (c *Client) likeMessage() {
	if c.currentUser == nil {
		fmt.Println("Please create a user first (command 1)")
		return
	}

	fmt.Print("Enter topic ID: ")
	topicIDStr, _ := c.reader.ReadString('\n')
	topicIDStr = strings.TrimSpace(topicIDStr)
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid topic ID")
		return
	}

	fmt.Print("Enter message ID: ")
	msgIDStr, _ := c.reader.ReadString('\n')
	msgIDStr = strings.TrimSpace(msgIDStr)
	msgID, err := strconv.ParseInt(msgIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid message ID")
		return
	}

	c.LikeMessage(topicID, msgID)
}

func (c *Client) LikeMessage(topicID int64, msgID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.headClient.LikeMessage(ctx, &razpravljalnica.LikeMessageRequest{
		TopicId:   topicID,
		MessageId: msgID,
		UserId:    c.currentUser.Id,
	})
	if err != nil {
		//fmt.Printf("Error liking message on HEAD: %v\n", err)
		return
	}

	//fmt.Printf("✓ Message liked on HEAD! Total likes: %d\n", message.Likes)
}

func (c *Client) subscribeTopic() {
	if c.currentUser == nil {
		fmt.Println("Please create a user first (command 1)")
		return
	}

	fmt.Print("Enter topic IDs (comma-separated): ")
	input, _ := c.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	topicIDStrs := strings.Split(input, ",")
	topicIDs := []int64{}
	lastMsgIDs := make(map[int64]int64)

	for _, idStr := range topicIDStrs {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			fmt.Println("Invalid topic ID:", idStr)
			return
		}
		topicIDs = append(topicIDs, id)

		// Pridobi zadnji ID sporočila tega topica iz TAIL (to bomo uporabili, da se ob subscriptionu na topic ne izpišejo vsi messages od topica za nazaj)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		msgs, err := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
			TopicId:       id,
			FromMessageId: 0,
			Limit:         1000000,
		})
		cancel()
		if err != nil || len(msgs.Messages) == 0 {
			lastMsgIDs[id] = 0
		} else {
			lastMsgIDs[id] = msgs.Messages[len(msgs.Messages)-1].Id
		}
	}

	// Pridobi assigned node od HEAD
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	subNodeResp, err := c.headClient.GetSubscriptionNode(ctx, &razpravljalnica.SubscriptionNodeRequest{
		UserId:  c.currentUser.Id,
		TopicId: topicIDs,
	})
	cancel()
	if err != nil {
		fmt.Printf("Error getting subscription node from HEAD: %v\n", err)
		return
	}

	fmt.Printf("✓ HEAD assigned subscription to node: %s @ %s\n",
		subNodeResp.Node.NodeId, subNodeResp.Node.Address)

	// Poveži se z assigned node
	subConn, err := grpc.Dial(subNodeResp.Node.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Error connecting to subscription node: %v\n", err)
		return
	}
	// defer subConn.Close() če naredimo to, se bo nehal izvajat in poslušat za te toppike

	subClient := razpravljalnica.NewMessageBoardClient(subConn)

	if c.subscriptions == nil {
		c.subscriptions = make(map[string]*Subscription)
	}
	// Gremo čez vse subscriptions
	for _, id := range topicIDs {
		// Odpri pretok za naročnino
		stream, err := subClient.SubscribeTopic(context.Background(), &razpravljalnica.SubscribeTopicRequest{
			TopicId: []int64{id},
			UserId:  c.currentUser.Id,
			// Da se izpišejo samo messages odkar smo se subscribali naprej, ne pa vsi messages za nazaj
			FromMessageId:  lastMsgIDs[id] + 1,
			SubscribeToken: subNodeResp.SubscribeToken,
		})
		if err != nil {
			fmt.Printf("Error subscribing to topics: %v\n", err)
			return
		}

		sub := &Subscription{
			Conn:   subConn,
			Client: subClient,
			Stream: stream,
			Done:   make(chan struct{}),
		}

		c.subMutex.Lock()
		c.subscriptions[strconv.FormatInt(id, 10)] = sub // shranimo subscription v client
		c.subMutex.Unlock()

		// zalaufamo poslušanje za topic
		go func(sub *Subscription) {
			for {
				select {
				case <-sub.Done:
					return
				default:
					event, err := sub.Stream.Recv()
					if err == io.EOF || err != nil {
						fmt.Printf("Error receiving event: %v\n", err)
						return
					}

					opName := "POST"
					if event.Op == razpravljalnica.OpType_OP_LIKE {
						opName = "LIKE"
					}
					fmt.Printf("\n[Event #%d - %s] Topic: %d, Message: %d, User: %d, Text: %s, Likes: %d\n",
						event.SequenceNumber, opName, event.Message.TopicId, event.Message.Id,
						event.Message.UserId, event.Message.Text, event.Message.Likes)
					fmt.Print("Enter command: ")
				}
			}
		}(sub)
	}

	fmt.Printf("✓ Subscribed to topics %v on node %s\n", topicIDs, subNodeResp.Node.NodeId)
	fmt.Println("Listening for events in background...")
}

func (c *Client) getClusterState() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state, err := c.ctrlClient.GetClusterState(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Printf("Error getting cluster state: %v\n", err)
		return
	}

	fmt.Printf("\n╔════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    Cluster State                       ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")
	if state.Head != nil {
		fmt.Printf("HEAD: %s @ %s\n", state.Head.NodeId, state.Head.Address)
	}
	if state.Tail != nil {
		fmt.Printf("TAIL: %s @ %s\n", state.Tail.NodeId, state.Tail.Address)
	}
	fmt.Printf("\nChain topology (%d nodes):\n", len(state.Chain))
	for i, node := range state.Chain {
		roleStr := "INTERMEDIATE"
		if node.Role == razpravljalnica.NodeRole_HEAD {
			roleStr = "HEAD"
		} else if node.Role == razpravljalnica.NodeRole_TAIL {
			roleStr = "TAIL"
		}
		fmt.Printf("  %d. [%s] %s @ %s\n", i+1, roleStr, node.NodeId, node.Address)
	}
}

func (c *Client) demoMode() {
	fmt.Println("\n╔════════════════════════════════════════════════════════╗")
	fmt.Println("║              Running Demo (Chain Replication)          ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")

	ctx := context.Background()

	// 1. Ustvari uporabnike na HEAD
	fmt.Println("\n1️⃣  Creating users on HEAD...")
	user1, _ := c.headClient.CreateUser(ctx, &razpravljalnica.CreateUserRequest{Name: "Alice"})
	user2, _ := c.headClient.CreateUser(ctx, &razpravljalnica.CreateUserRequest{Name: "Bob"})
	fmt.Printf("   ✓ Created: %s (ID: %d), %s (ID: %d)\n", user1.Name, user1.Id, user2.Name, user2.Id)
	c.currentUser = user1
	time.Sleep(500 * time.Millisecond)

	// 2. Ustvari teme na HEAD
	fmt.Println("\n2️⃣  Creating topics on HEAD...")
	topic1, _ := c.headClient.CreateTopic(ctx, &razpravljalnica.CreateTopicRequest{Name: "Programiranje"})
	topic2, _ := c.headClient.CreateTopic(ctx, &razpravljalnica.CreateTopicRequest{Name: "Šport"})
	fmt.Printf("   ✓ Created: %s (ID: %d), %s (ID: %d)\n", topic1.Name, topic1.Id, topic2.Name, topic2.Id)
	time.Sleep(500 * time.Millisecond)

	// 3. Počakaj na replikacijo
	fmt.Println("\n⏳ Waiting for replication to TAIL...")
	time.Sleep(1 * time.Second)

	// 4. Preberi teme iz TAIL
	fmt.Println("\n3️⃣  Reading topics from TAIL...")
	topics, _ := c.tailClient.ListTopics(ctx, &emptypb.Empty{})
	fmt.Printf("   ✓ TAIL has %d topics\n", len(topics.Topics))
	for _, t := range topics.Topics {
		fmt.Printf("     - [%d] %s\n", t.Id, t.Name)
	}

	// 5. Objavi sporočila na HEAD
	fmt.Println("\n4️⃣  Posting messages to HEAD...")
	msg1, _ := c.headClient.PostMessage(ctx, &razpravljalnica.PostMessageRequest{
		TopicId: topic1.Id,
		UserId:  user1.Id,
		Text:    "Go je odličen jezik za verižno replikacijo!",
	})
	msg2, _ := c.headClient.PostMessage(ctx, &razpravljalnica.PostMessageRequest{
		TopicId: topic1.Id,
		UserId:  user2.Id,
		Text:    "gRPC omogoča hitro komunikacijo!",
	})
	fmt.Printf("   ✓ Posted %d messages to HEAD\n", 2)
	time.Sleep(1 * time.Second)

	// 6. Preberi sporočila iz TAIL
	fmt.Println("\n5️⃣  Reading messages from TAIL...")
	messages, _ := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
		TopicId:       topic1.Id,
		FromMessageId: 0,
		Limit:         0,
	})
	fmt.Printf("   ✓ TAIL has %d messages in topic '%s':\n", len(messages.Messages), topic1.Name)
	for _, msg := range messages.Messages {
		fmt.Printf("     [%d] User %d: %s\n", msg.Id, msg.UserId, msg.Text)
	}

	// 7. Všečkaj sporočila na HEAD
	fmt.Println("\n6️⃣  Liking messages on HEAD...")
	c.headClient.LikeMessage(ctx, &razpravljalnica.LikeMessageRequest{
		TopicId:   topic1.Id,
		MessageId: msg1.Id,
		UserId:    user2.Id,
	})
	c.headClient.LikeMessage(ctx, &razpravljalnica.LikeMessageRequest{
		TopicId:   topic1.Id,
		MessageId: msg2.Id,
		UserId:    user1.Id,
	})
	fmt.Printf("   ✓ Liked 2 messages on HEAD\n")
	time.Sleep(1 * time.Second)

	// 8. Preveri všečke iz TAIL
	fmt.Println("\n7️⃣  Verifying likes from TAIL...")
	messagesWithLikes, _ := c.tailClient.GetMessages(ctx, &razpravljalnica.GetMessagesRequest{
		TopicId:       topic1.Id,
		FromMessageId: 0,
		Limit:         0,
	})
	fmt.Printf("   ✓ Messages with likes (from TAIL):\n")
	for _, msg := range messagesWithLikes.Messages {
		fmt.Printf("     [%d] Likes: %d\n", msg.Id, msg.Likes)
	}

	// 9. Prikaži stanje klastra
	fmt.Println("\n8️⃣  Cluster state:")
	c.getClusterState()

	fmt.Println("\n╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Demo Complete!                      ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println("\n✅ All operations demonstrated:")
	fmt.Println("   • Writes go to HEAD")
	fmt.Println("   • Reads go to TAIL")
	fmt.Println("   • Data replicated through chain")
	fmt.Println("   • Subscriptions load-balanced by HEAD")
}

func ClientMain(headURL, tailURL string) {
	client, err := NewClient(headURL, tailURL)
	if err != nil {
		panic(err)
	}
	client.Run()
}
