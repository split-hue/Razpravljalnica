package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"razpravljalnica/razpravljalnica"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// strkt. za shranjevanje pod v pom
type Storage struct {
	users           map[int64]*razpravljalnica.User
	topics          map[int64]*razpravljalnica.Topic
	messages        map[int64]*razpravljalnica.Message
	messagesByTopic map[int64][]*razpravljalnica.Message
	likes           map[string]bool //enoličn kl. da nau duplikatou
	// i++ za vse da ve ke štartat naprej
	nextUserID    int64
	nextTopicID   int64
	nextMessageID int64
	subscribers   map[string]*Subscriber //naročniki po ID
	sequenceNum   int64                  //globalna sek. št. op.
	mu            sync.RWMutex
}

type Subscriber struct {
	topicIDs  []int64
	userID    int64
	fromMsgID int64
	eventChan chan *razpravljalnica.MessageEvent
	ctx       context.Context //kontekst streama za prekinitve
}

func NewStorage() *Storage { //konstruktor za inicializacijo Storage
	return &Storage{
		users:           make(map[int64]*razpravljalnica.User),
		topics:          make(map[int64]*razpravljalnica.Topic),
		messages:        make(map[int64]*razpravljalnica.Message),
		messagesByTopic: make(map[int64][]*razpravljalnica.Message),
		likes:           make(map[string]bool),
		subscribers:     make(map[string]*Subscriber),
		nextUserID:      1,
		nextTopicID:     1,
		nextMessageID:   1,
		sequenceNum:     1,
	}
}

type ChainNode struct { //predstavlja vozlišče v verigi
	razpravljalnica.UnimplementedMessageBoardServer
	razpravljalnica.UnimplementedControlPlaneServer
	razpravljalnica.UnimplementedChainReplicationServer

	nodeID  string
	address string
	role    razpravljalnica.NodeRole
	storage *Storage

	// Chain topology
	successor   *NodeConnection //pov na naslednga v verigi
	predecessor *NodeConnection //-||- predhodnga, sam ne uporabm
	allNodes    []*razpravljalnica.NodeInfo

	// Subscription load balancing //counter za round-robin razporeditev naročnin
	subscriptionRoundRobin uint64

	mu sync.RWMutex
}

type NodeConnection struct {
	nodeInfo *razpravljalnica.NodeInfo
	conn     *grpc.ClientConn //gRPC pov
	client   razpravljalnica.ChainReplicationClient
}

func NewChainNode(nodeID, address string, role razpravljalnica.NodeRole) *ChainNode {
	return &ChainNode{
		nodeID:   nodeID,
		address:  address,
		role:     role,
		storage:  NewStorage(),
		allNodes: []*razpravljalnica.NodeInfo{},
	}
}

// SetSuccessor nastavi naslednika v verigi
func (n *ChainNode) SetSuccessor(nodeInfo *razpravljalnica.NodeInfo) error {
	conn, err := grpc.Dial(nodeInfo.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.successor = &NodeConnection{
		nodeInfo: nodeInfo,
		conn:     conn,
		client:   razpravljalnica.NewChainReplicationClient(conn),
	}

	fmt.Printf("✓ Set successor: %s @ %s\n", nodeInfo.NodeId, nodeInfo.Address)
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Client-facing API (MessageBoard) za odjemalce
////////////////////////////////////////////////////////////////////////////////

// CreateUser - samo na HEAD
func (n *ChainNode) CreateUser(ctx context.Context, req *razpravljalnica.CreateUserRequest) (*razpravljalnica.User, error) {
	if n.role != razpravljalnica.NodeRole_HEAD {
		return nil, fmt.Errorf("write operations must go to HEAD node")
	}

	n.storage.mu.Lock()
	user := &razpravljalnica.User{
		Id:   n.storage.nextUserID,
		Name: req.Name,
	}
	n.storage.users[user.Id] = user
	n.storage.nextUserID++
	seqNum := n.storage.sequenceNum
	n.storage.sequenceNum++
	n.storage.mu.Unlock()

	// Propagate to chain
	if n.successor != nil {
		writeOp := &razpravljalnica.WriteOperation{
			SequenceNumber: seqNum,
			OpType:         razpravljalnica.OpType_OP_POST,
			Operation:      &razpravljalnica.WriteOperation_CreateUser{CreateUser: req},
			Timestamp:      timestamppb.Now(),
		}
		go n.propagateWrite(writeOp)
	}

	fmt.Printf("[HEAD] Created user: %s (ID: %d)\n", user.Name, user.Id)
	return user, nil
}

// CreateTopic - samo na HEAD
func (n *ChainNode) CreateTopic(ctx context.Context, req *razpravljalnica.CreateTopicRequest) (*razpravljalnica.Topic, error) {
	if n.role != razpravljalnica.NodeRole_HEAD {
		return nil, fmt.Errorf("write operations must go to HEAD node")
	}

	n.storage.mu.Lock()
	topic := &razpravljalnica.Topic{
		Id:   n.storage.nextTopicID,
		Name: req.Name,
	}
	n.storage.topics[topic.Id] = topic
	n.storage.messagesByTopic[topic.Id] = []*razpravljalnica.Message{}
	n.storage.nextTopicID++
	seqNum := n.storage.sequenceNum
	n.storage.sequenceNum++
	n.storage.mu.Unlock()

	// Propagate to chain
	if n.successor != nil {
		writeOp := &razpravljalnica.WriteOperation{
			SequenceNumber: seqNum,
			OpType:         razpravljalnica.OpType_OP_POST,
			Operation:      &razpravljalnica.WriteOperation_CreateTopic{CreateTopic: req},
			Timestamp:      timestamppb.Now(),
		}
		go n.propagateWrite(writeOp)
	}

	fmt.Printf("[HEAD] Created topic: %s (ID: %d)\n", topic.Name, topic.Id)
	return topic, nil
}

// PostMessage - samo na HEAD
func (n *ChainNode) PostMessage(ctx context.Context, req *razpravljalnica.PostMessageRequest) (*razpravljalnica.Message, error) {
	if n.role != razpravljalnica.NodeRole_HEAD {
		return nil, fmt.Errorf("write operations must go to HEAD node")
	}

	n.storage.mu.Lock()
	if _, exists := n.storage.users[req.UserId]; !exists {
		n.storage.mu.Unlock()
		return nil, fmt.Errorf("user with ID %d does not exist", req.UserId)
	}
	if _, exists := n.storage.topics[req.TopicId]; !exists {
		n.storage.mu.Unlock()
		return nil, fmt.Errorf("topic with ID %d does not exist", req.TopicId)
	}

	message := &razpravljalnica.Message{
		Id:        n.storage.nextMessageID,
		TopicId:   req.TopicId,
		UserId:    req.UserId,
		Text:      req.Text,
		CreatedAt: timestamppb.Now(),
		Likes:     0,
	}
	n.storage.messages[message.Id] = message
	n.storage.messagesByTopic[req.TopicId] = append(n.storage.messagesByTopic[req.TopicId], message)
	n.storage.nextMessageID++
	seqNum := n.storage.sequenceNum
	n.storage.sequenceNum++
	n.storage.mu.Unlock()

	// Propagate to chain
	if n.successor != nil {
		writeOp := &razpravljalnica.WriteOperation{
			SequenceNumber: seqNum,
			OpType:         razpravljalnica.OpType_OP_POST,
			Operation:      &razpravljalnica.WriteOperation_PostMessage{PostMessage: req},
			Timestamp:      timestamppb.Now(),
		}
		go n.propagateWrite(writeOp)
	}

	// Notify local subscribers
	event := &razpravljalnica.MessageEvent{
		SequenceNumber: seqNum,
		Op:             razpravljalnica.OpType_OP_POST,
		Message:        message,
		EventAt:        timestamppb.Now(),
	}
	n.notifySubscribers(event)

	fmt.Printf("[HEAD] Posted message to topic %d: %s\n", req.TopicId, req.Text)
	return message, nil
}

// LikeMessage - samo na HEAD
func (n *ChainNode) LikeMessage(ctx context.Context, req *razpravljalnica.LikeMessageRequest) (*razpravljalnica.Message, error) {
	if n.role != razpravljalnica.NodeRole_HEAD {
		return nil, fmt.Errorf("write operations must go to HEAD node")
	}

	n.storage.mu.Lock()
	message, exists := n.storage.messages[req.MessageId]
	if !exists {
		n.storage.mu.Unlock()
		return nil, fmt.Errorf("message with ID %d does not exist", req.MessageId)
	}

	likeKey := fmt.Sprintf("%d_%d_%d", req.TopicId, req.MessageId, req.UserId)
	if n.storage.likes[likeKey] {
		n.storage.mu.Unlock()
		return message, nil
	}

	n.storage.likes[likeKey] = true
	message.Likes++
	seqNum := n.storage.sequenceNum
	n.storage.sequenceNum++
	n.storage.mu.Unlock()

	// Propagate to chain
	if n.successor != nil {
		writeOp := &razpravljalnica.WriteOperation{
			SequenceNumber: seqNum,
			OpType:         razpravljalnica.OpType_OP_LIKE,
			Operation:      &razpravljalnica.WriteOperation_LikeMessage{LikeMessage: req},
			Timestamp:      timestamppb.Now(),
		}
		go n.propagateWrite(writeOp)
	}

	// Notify local subscribers
	event := &razpravljalnica.MessageEvent{
		SequenceNumber: seqNum,
		Op:             razpravljalnica.OpType_OP_LIKE,
		Message:        message,
		EventAt:        timestamppb.Now(),
	}
	n.notifySubscribers(event)

	fmt.Printf("[HEAD] Liked message %d (now has %d likes)\n", req.MessageId, message.Likes)
	return message, nil
}

// GetSubscriptionNode - HEAD določi vozlišče za subscription (load balancing)
func (n *ChainNode) GetSubscriptionNode(ctx context.Context, req *razpravljalnica.SubscriptionNodeRequest) (*razpravljalnica.SubscriptionNodeResponse, error) {
	if n.role != razpravljalnica.NodeRole_HEAD {
		return nil, fmt.Errorf("subscription node assignment only from HEAD")
	}

	token := generateToken()

	n.mu.RLock()
	nodeCount := len(n.allNodes)
	n.mu.RUnlock()

	if nodeCount == 0 {
		// Samo HEAD vozlišče
		return &razpravljalnica.SubscriptionNodeResponse{
			SubscribeToken: token,
			Node: &razpravljalnica.NodeInfo{
				NodeId:  n.nodeID,
				Address: n.address,
				Role:    n.role,
			},
		}, nil
	}

	// Round-robin load balancing
	idx := atomic.AddUint64(&n.subscriptionRoundRobin, 1) % uint64(nodeCount)

	n.mu.RLock()
	assignedNode := n.allNodes[idx]
	n.mu.RUnlock()

	fmt.Printf("[HEAD] Assigned subscription to node %s (index %d of %d)\n",
		assignedNode.NodeId, idx, nodeCount)

	return &razpravljalnica.SubscriptionNodeResponse{
		SubscribeToken: token,
		Node:           assignedNode,
	}, nil
}

// ListTopics - samo na TAIL
func (n *ChainNode) ListTopics(ctx context.Context, req *emptypb.Empty) (*razpravljalnica.ListTopicsResponse, error) {
	if n.role != razpravljalnica.NodeRole_TAIL {
		return nil, fmt.Errorf("read operations must go to TAIL node")
	}

	n.storage.mu.RLock()
	defer n.storage.mu.RUnlock()

	topics := make([]*razpravljalnica.Topic, 0, len(n.storage.topics))
	for _, topic := range n.storage.topics {
		topics = append(topics, topic)
	}

	fmt.Printf("[TAIL] Listed %d topics\n", len(topics))
	return &razpravljalnica.ListTopicsResponse{Topics: topics}, nil
}

// GetMessages - samo na TAIL
func (n *ChainNode) GetMessages(ctx context.Context, req *razpravljalnica.GetMessagesRequest) (*razpravljalnica.GetMessagesResponse, error) {
	if n.role != razpravljalnica.NodeRole_TAIL {
		return nil, fmt.Errorf("read operations must go to TAIL node")
	}

	n.storage.mu.RLock()
	defer n.storage.mu.RUnlock()

	allMessages, exists := n.storage.messagesByTopic[req.TopicId]
	if !exists {
		return &razpravljalnica.GetMessagesResponse{Messages: []*razpravljalnica.Message{}}, nil
	}

	messages := []*razpravljalnica.Message{}
	for _, msg := range allMessages {
		if msg.Id >= req.FromMessageId {
			messages = append(messages, msg)
		}
	}

	if req.Limit > 0 && int32(len(messages)) > req.Limit {
		messages = messages[:req.Limit]
	}

	fmt.Printf("[TAIL] Retrieved %d messages from topic %d\n", len(messages), req.TopicId)
	return &razpravljalnica.GetMessagesResponse{Messages: messages}, nil
}

// SubscribeTopic - lahko na kateremkoli vozlišču
func (n *ChainNode) SubscribeTopic(req *razpravljalnica.SubscribeTopicRequest, stream grpc.ServerStreamingServer[razpravljalnica.MessageEvent]) error {
	n.storage.mu.Lock()

	subscriber := &Subscriber{
		topicIDs:  req.TopicId,
		userID:    req.UserId,
		fromMsgID: req.FromMessageId,
		eventChan: make(chan *razpravljalnica.MessageEvent, 100),
		ctx:       stream.Context(),
	}

	subID := generateToken()
	n.storage.subscribers[subID] = subscriber
	n.storage.mu.Unlock()

	fmt.Printf("[%s] User %d subscribed to topics %v\n", n.nodeID, req.UserId, req.TopicId)

	// Pošlji zgodovino sporočil
	n.storage.mu.RLock()
	for _, topicID := range req.TopicId {
		if messages, exists := n.storage.messagesByTopic[topicID]; exists {
			for _, msg := range messages {
				if msg.Id >= req.FromMessageId {
					event := &razpravljalnica.MessageEvent{
						SequenceNumber: 0,
						Op:             razpravljalnica.OpType_OP_POST,
						Message:        msg,
						EventAt:        msg.CreatedAt,
					}
					if err := stream.Send(event); err != nil {
						n.storage.mu.RUnlock()
						return err
					}
				}
			}
		}
	}
	n.storage.mu.RUnlock()

	//posluši nove dogodke
	defer func() {
		n.storage.mu.Lock()
		delete(n.storage.subscribers, subID)
		close(subscriber.eventChan)
		n.storage.mu.Unlock()
	}()

	for {
		select {
		case event := <-subscriber.eventChan:
			if err := stream.Send(event); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Control Plane lmao
// //////////////////////////////////////////////////////////////////////////////
// mož. 8, pove specs
func (n *ChainNode) GetClusterState(ctx context.Context, req *emptypb.Empty) (*razpravljalnica.GetClusterStateResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var head, tail *razpravljalnica.NodeInfo //iz meta pod. prebere vn kera sta
	for _, node := range n.allNodes {        //poišče head in tail v seznamu
		if node.Role == razpravljalnica.NodeRole_HEAD {
			head = node
		}
		if node.Role == razpravljalnica.NodeRole_TAIL {
			tail = node
		}
	}

	return &razpravljalnica.GetClusterStateResponse{
		Head:  head,
		Tail:  tail,
		Chain: n.allNodes,
	}, nil
}

//func (n *ChainNode) RegisterNode(ctx context.Context, req *razpravljalnica.RegisterNodeRequest) (*razpravljalnica.RegisterNodeResponse, error) {
//čeb hotle da za dinamično dodajanje vozlišč ampak taj še daleč
//	return &razpravljalnica.RegisterNodeResponse{Success: false}, nil
//}

////////////////////////////////////////////////////////////////////////////////
// Chain Replication (node-to-node)
////////////////////////////////////////////////////////////////////////////////

func (n *ChainNode) PropagateWrite(ctx context.Context, op *razpravljalnica.WriteOperation) (*razpravljalnica.WriteAck, error) {
	//izvedi operacijo lokalno
	err := n.applyWrite(op) //ta applywrite je lok.
	if err != nil {
		return &razpravljalnica.WriteAck{
			SequenceNumber: op.SequenceNumber,
			Success:        false,
			Error:          err.Error(),
		}, nil
	}
	//če nismo tail, propagiraj naprej
	if n.successor != nil {
		_, err := n.successor.client.PropagateWrite(ctx, op)
		if err != nil {
			fmt.Printf("[%s] Error propagating to successor: %v\n", n.nodeID, err)
		}
	}

	return &razpravljalnica.WriteAck{
		SequenceNumber: op.SequenceNumber,
		Success:        true,
	}, nil
}

func (n *ChainNode) AckWrite(ctx context.Context, ack *razpravljalnica.WriteAck) (*emptypb.Empty, error) {
	//ACK prejeto od naslednika
	return &emptypb.Empty{}, nil
}

func (n *ChainNode) Heartbeat(ctx context.Context, req *razpravljalnica.HeartbeatRequest) (*razpravljalnica.HeartbeatResponse, error) {
	n.storage.mu.RLock()
	lastSeq := n.storage.sequenceNum
	n.storage.mu.RUnlock()

	return &razpravljalnica.HeartbeatResponse{
		NodeId:       n.nodeID,
		Alive:        true,
		LastSequence: lastSeq,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Helper functions
////////////////////////////////////////////////////////////////////////////////

func (n *ChainNode) propagateWrite(op *razpravljalnica.WriteOperation) {
	if n.successor == nil { //če ni naslednika, nč ne propagiraj naprej
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //timeout za RPC
	defer cancel()

	_, err := n.successor.client.PropagateWrite(ctx, op) //pošili naprej :))
	if err != nil {
		fmt.Printf("[%s] Failed to propagate write seq=%d: %v\n", n.nodeID, op.SequenceNumber, err)
	}
}

// +++++++++++++++++++++++++++
func (n *ChainNode) applyWrite(op *razpravljalnica.WriteOperation) error {
	n.storage.mu.Lock() //zaklen k bomo pisal
	defer n.storage.mu.Unlock()

	switch operation := op.Operation.(type) {
	case *razpravljalnica.WriteOperation_CreateUser:
		req := operation.CreateUser
		user := &razpravljalnica.User{
			Id:   n.storage.nextUserID,
			Name: req.Name,
		}
		n.storage.users[user.Id] = user //shrani kopijo uporabnika
		n.storage.nextUserID++
		fmt.Printf("[%s] Replicated CreateUser: %s (ID: %d)\n", n.nodeID, user.Name, user.Id)

	case *razpravljalnica.WriteOperation_CreateTopic:
		req := operation.CreateTopic
		topic := &razpravljalnica.Topic{
			Id:   n.storage.nextTopicID,
			Name: req.Name,
		}
		n.storage.topics[topic.Id] = topic //-||- teme
		n.storage.messagesByTopic[topic.Id] = []*razpravljalnica.Message{}
		n.storage.nextTopicID++
		fmt.Printf("[%s] Replicated CreateTopic: %s (ID: %d)\n", n.nodeID, topic.Name, topic.Id)

	case *razpravljalnica.WriteOperation_PostMessage:
		req := operation.PostMessage
		message := &razpravljalnica.Message{
			Id:        n.storage.nextMessageID,
			TopicId:   req.TopicId,
			UserId:    req.UserId,
			Text:      req.Text,
			CreatedAt: op.Timestamp,
			Likes:     0,
		}
		n.storage.messages[message.Id] = message
		n.storage.messagesByTopic[req.TopicId] = append(n.storage.messagesByTopic[req.TopicId], message)
		n.storage.nextMessageID++

		// Notify local subscribers
		event := &razpravljalnica.MessageEvent{
			SequenceNumber: op.SequenceNumber,
			Op:             razpravljalnica.OpType_OP_POST,
			Message:        message,
			EventAt:        op.Timestamp,
		}
		n.notifySubscribersLocked(event)

		fmt.Printf("[%s] Replicated PostMessage to topic %d\n", n.nodeID, req.TopicId)

	case *razpravljalnica.WriteOperation_LikeMessage:
		req := operation.LikeMessage
		likeKey := fmt.Sprintf("%d_%d_%d", req.TopicId, req.MessageId, req.UserId)
		if !n.storage.likes[likeKey] {
			n.storage.likes[likeKey] = true
			if msg, exists := n.storage.messages[req.MessageId]; exists {
				msg.Likes++

				// Notify local subscribers
				event := &razpravljalnica.MessageEvent{
					SequenceNumber: op.SequenceNumber,
					Op:             razpravljalnica.OpType_OP_LIKE,
					Message:        msg,
					EventAt:        op.Timestamp,
				}
				n.notifySubscribersLocked(event)
			}
		}
		fmt.Printf("[%s] Replicated LikeMessage: msg=%d\n", n.nodeID, req.MessageId)
	}

	return nil
} //+++++++++++++++++++++++++++

func (n *ChainNode) notifySubscribers(event *razpravljalnica.MessageEvent) {
	n.storage.mu.Lock()
	defer n.storage.mu.Unlock()
	n.notifySubscribersLocked(event)
}

func (n *ChainNode) notifySubscribersLocked(event *razpravljalnica.MessageEvent) {
	for _, sub := range n.storage.subscribers { //iteriraj po vseh subscriberjih
		for _, topicID := range sub.topicIDs { //preveri čej subscriber naročen na to temo
			if topicID == event.Message.TopicId {
				select {
				case sub.eventChan <- event: //poskus poslati event
				default: //če kanal poln -> preskoči
				}
				break //vn iz topicIDs
			}
		}
	}
}

// generira rand 16-bajtni hex token
func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func Server(url string, role razpravljalnica.NodeRole, successorAddr string, allNodeAddrs []string) {
	hostname, err := os.Hostname() //dubi hostname stroja
	if err != nil {
		panic(err)
	}

	nodeID := fmt.Sprintf("node_%s_%s", hostname, url)
	node := NewChainNode(nodeID, url, role)

	//nastau verižno topologijo
	if successorAddr != "" {
		successorInfo := &razpravljalnica.NodeInfo{
			NodeId:  fmt.Sprintf("successor_%s", successorAddr),
			Address: successorAddr,
		}
		if err := node.SetSuccessor(successorInfo); err != nil {
			fmt.Printf("XX Failed to connect to successor: %v\n", err)
		}
	}

	//shrani info o vseh vozliščih-za load balancing
	node.mu.Lock()
	for i, addr := range allNodeAddrs {
		var nodeRole razpravljalnica.NodeRole
		if i == 0 {
			nodeRole = razpravljalnica.NodeRole_HEAD
		} else if i == len(allNodeAddrs)-1 {
			nodeRole = razpravljalnica.NodeRole_TAIL
		} else {
			nodeRole = razpravljalnica.NodeRole_INTERMEDIATE
		}

		node.allNodes = append(node.allNodes, &razpravljalnica.NodeInfo{
			NodeId:  fmt.Sprintf("node_%d", i),
			Address: addr,
			Role:    nodeRole,
		})
	}
	node.mu.Unlock()

	grpcServer := grpc.NewServer()

	razpravljalnica.RegisterMessageBoardServer(grpcServer, node)
	razpravljalnica.RegisterControlPlaneServer(grpcServer, node)
	razpravljalnica.RegisterChainReplicationServer(grpcServer, node)

	listener, err := net.Listen("tcp", url)
	if err != nil {
		panic(err)
	}

	roleStr := "INTERMEDIATE"
	if role == razpravljalnica.NodeRole_HEAD {
		roleStr = "HEAD"
	} else if role == razpravljalnica.NodeRole_TAIL {
		roleStr = "TAIL"
	}

	fmt.Printf("╔════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  gRPC Razpravljalnica [%s] listening at %s%s\n", roleStr, hostname, url)
	fmt.Printf("║  Node ID: %s\n", nodeID)
	if successorAddr != "" {
		fmt.Printf("║  Successor: %s\n", successorAddr)
	}
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")

	if err := grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}
