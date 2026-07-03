package ta

import (
	"context"
	"encoding/binary"
	"log"
	"sync"

	pb "github.com/free5gc/udr/internal/ta-pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	taClient pb.TrustAnchorClient
	ownerID  uint64
	mu       sync.Mutex
	ctx      context.Context
)

func Init(address string) error {
	// Create a grpc connection
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	// Create a new client in ta
	taClient = pb.NewTrustAnchorClient(conn)

	// Add the client as owner to ta db
	res, err := taClient.AddOwner(context.Background(), &pb.AddOwner{})
	if err != nil {
		return err
	}

	ownerID = res.OwnerId

	// Package ownerID as metadata
	ownerBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(ownerBytes, ownerID)
	ctx = metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))

	log.Printf("[TA Client] [Owner ID: %d] Connected to TA successfully.", ownerID)
	return nil
}
