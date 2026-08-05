package ta

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/free5gc/webconsole/backend/logger"
	pb "github.com/free5gc/webconsole/backend/ta-pb"
	"go.mongodb.org/mongo-driver/bson"
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

func TaInit(address string) error {
	mu.Lock()
	defer mu.Unlock()

	var lastErr error
	if taClient != nil {
		logger.InitLog.Warnln("A TaClient connection already exists!")
		return nil
	}

	for i := 1; i <= max_retries; i++ {
		c, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			lastErr = fmt.Errorf("Failed to create GRPC connection: %w. Restarting connection...", err)
			time.Sleep(retry_delay)
			continue
		}

		conn = c
		taClient = pb.NewTrustAnchorClient(conn)

		res, err := taClient.AddOwner(context.Background(), &pb.AddOwner{})
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("Failed to register owner: %w. Restarting connection...", err)
			time.Sleep(retry_delay)
			continue
		}

		ownerID = res.OwnerId

		ownerBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(ownerBytes, ownerID)
		ctx = metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))

		logger.InitLog.Infof("[TA Client] [Owner ID: %d] Connected to TA successfully. Attempts: %d", ownerID, i)
		return nil
	}

	return fmt.Errorf("Failed to initialise TA-Client after %d attempts: %w", max_retries, lastErr)
}

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

func TaWrite(collName string, key string, value interface{}) error {
	mu.Lock()
	client := taClient
	mu.Unlock()

	if client == nil {
		return fmt.Errorf("Trust anchor client is not initialized")
	}

	category := TaGetCategory(collName)

	valBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("Failed to serialize value to JSON: %w", err)
	}

	req := &pb.Write{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(key),
		},
		Value: valBytes,
	}

	_, err = client.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC write to Trust Anchor failed: %w", err)
	}

	return nil
}

func TaGetCategory(collName string) byte {
	switch {
	case collName == "applicationData.influenceData":
		return CategoryInfluenceData
	case collName == "applicationData.pfds":
		return CategoryPdf
	case len(collName) >= 11 && collName[:11] == "policyData.":
		return CategoryPolicyData
	case len(collName) >= 17 && collName[:17] == "subscriptionData.":
		return CategorySubscriptionData
	default:
		return CategoryDefault
	}
}

func TaExtractfromFilter(filter bson.M) string {
	if filter == nil {
		return ""
	}

	if ueId, ok := filter["ueId"].(string); ok {
		if servingPlmnId, ok := filter["servingPlmnId"].(string); ok {
			return fmt.Sprintf("%s_%s", ueId, servingPlmnId)
		}
		if pduSessionId, ok := filter["pduSessionId"]; ok {
			return fmt.Sprintf("%s_%v", ueId, pduSessionId)
		}
		if limitId, ok := filter["limitId"].(string); ok {
			return fmt.Sprintf("%s_%s", ueId, limitId)
		}
		if usageMonId, ok := filter["usageMonId"].(string); ok {
			return fmt.Sprintf("%s_%s", ueId, usageMonId)
		}
		return ueId
	}

	if influenceId, ok := filter["influenceId"].(string); ok {
		return influenceId
	}
	if sharedDataId, ok := filter["sharedDataId"].(string); ok {
		return sharedDataId
	}
	if applicationId, ok := filter["applicationId"].(string); ok {
		return applicationId
	}
	if bdtReferenceId, ok := filter["bdtReferenceId"].(string); ok {
		return bdtReferenceId
	}

	return fmt.Sprintf("%v", filter)
}
