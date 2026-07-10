package ta

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/free5gc/udr/internal/ta-pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	conn     *grpc.ClientConn
	taClient pb.TrustAnchorClient
	ownerID  uint64
	mu       sync.Mutex
	ctx      context.Context
)

const (
	CategoryDefault          byte = 0x00
	CategoryInfluenceData    byte = 0x01
	CategoryPdf              byte = 0x02
	CategorySubscriptionData byte = 0x03
	CategoryPolicyData       byte = 0x04
)

var max_retries = 5
var retry_delay = 2 * time.Second
var lastErr error

func TaInit(address string) error {
	mu.Lock()
	defer mu.Unlock()

	// To avoid duplicate connection
	if taClient != nil {
		fmt.Println("A TaClient connection already exists!")
		return nil
	}

	for i := 1; i <= max_retries; i++ {
		// Create a grpc connection
		c, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			lastErr = fmt.Errorf("Failed to create GRPC connection: %w. Restarting the connection ...", err)
			time.Sleep(retry_delay)
			continue
		}

		conn = c
		// Create a new client in ta
		taClient = pb.NewTrustAnchorClient(conn)

		// Add the client as owner to ta db
		res, err := taClient.AddOwner(context.Background(), &pb.AddOwner{})
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("Failed to register owner: %w. Restarting the connection ...", err)
			time.Sleep(retry_delay)
			continue
		}

		ownerID = res.OwnerId

		// Package ownerID as metadata
		ownerBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(ownerBytes, ownerID)
		ctx = metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))

		log.Printf("[TA Client] [Owner ID: %d] Connected to TA successfully. Attempts: %d", ownerID, i)
		return nil
	}

	return fmt.Errorf("Failed to initialise TA-Client after %d attempts: %w", max_retries, lastErr)
}

// Close GRPC connection and empty the variables
func TaClose() error {
	mu.Lock()
	defer mu.Unlock()

	if conn != nil {
		err := conn.Close()

		conn = nil
		taClient = nil
		return err
	}
	return nil
}

// Mirror writing operation to TA
func TaWrite(category byte, key string, value interface{}) error {
	if taClient == nil {
		return fmt.Errorf("Trust anchor client is not initialized")
	}

	// Serialise value structure/map to JSON bytes
	valBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("Failed to serialize value to JSON: %w", err)
	}

	// Create a request struct to write the data
	req := &pb.Write{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(key),
		},
		Value: valBytes,
	}

	_, err = taClient.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC write to Trust Anchor failed: %w", err)
	}

	return nil
}

func TaGetCategory(collName string) byte {
	switch {
	case collName == "applicationData.influenceData":
		return CategoryInfluenceData
	case collName == "application.pfds":
		return CategoryPdf
	case len(collName) >= 11 && collName[:11] == "policyData.":
		return CategoryPolicyData
	case len(collName) >= 11 && collName[:11] == "subscriptionData.":
		return CategorySubscriptionData
	default:
		return CategoryDefault
	}
}
