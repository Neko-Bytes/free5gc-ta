package ta

import (
	"context"
	"encoding/binary"

	// "encoding/json"
	"fmt"
	// "strings"
	"sync"
	"time"

	"github.com/free5gc/udr/internal/logger"
	pb "github.com/free5gc/udr/internal/ta-pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type TrustAnchorConnector struct {
	conn    *grpc.ClientConn
	client  pb.TrustAnchorClient
	ownerID uint64
	taID    string
	mu      sync.Mutex
	ctx     context.Context

	// Memory cache
	ueOwnerCache   map[string]uint64
	ueOwnerCacheMu sync.RWMutex // Using RWMutex to allow multiple reads.
}

var taConnector = &TrustAnchorConnector{
	ueOwnerCache: make(map[string]uint64),
}

// GetTaID returns the local Trust Anchor ID persisted at init time.
// It is used as Location.TaId for local owner data.
func GetTaID() string {
	taConnector.mu.Lock()
	defer taConnector.mu.Unlock()
	return taConnector.taID
}

const (
	CategoryDefault          byte = 0x00
	CategoryInfluenceData    byte = 0x01
	CategoryPdf              byte = 0x02
	CategorySubscriptionData byte = 0x03
	CategoryPolicyData       byte = 0x04
	CategoryPhonebook        byte = 0x10
)

// systemOwnerID is the TA-reserved owner space shared with Webconsole (admin-owner-id: 1).
const systemOwnerID uint64 = 1

var max_retries = 30
var retry_delay = 2 * time.Second

func TaInit(address string) error {
	taConnector.mu.Lock()
	defer taConnector.mu.Unlock()

	var lastErr error
	// To avoid duplicate connection
	if taConnector.client != nil {
		logger.InitLog.Warnln("A TrustAnchorConnector connection already exists!")
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

		taConnector.conn = c
		// Create a new client in ta
		taConnector.client = pb.NewTrustAnchorClient(taConnector.conn)

		// Connect as system Owner 1 instead of registering a new owner via AddOwner.
		taConnector.ownerID = systemOwnerID

		idRes, err := taConnector.client.GetTaId(context.Background(), &pb.Empty{})
		if err != nil || idRes.GetTaId() == "" {
			_ = taConnector.conn.Close()
			taConnector.conn = nil
			taConnector.client = nil
			if err == nil {
				err = fmt.Errorf("empty TA ID returned")
			}
			lastErr = fmt.Errorf("Failed to fetch TA ID: %w. Restarting the connection ...", err)
			time.Sleep(retry_delay)
			continue
		}
		taConnector.taID = idRes.GetTaId()

		// Package ownerID as metadata
		ownerBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(ownerBytes, taConnector.ownerID)
		taConnector.ctx = metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))

		// Fail fast if system Owner 1 does not exist.
		if _, err := taConnector.client.GetMap(taConnector.ctx, &pb.GetMap{}); err != nil {
			_ = taConnector.conn.Close()
			taConnector.conn = nil
			taConnector.client = nil
			lastErr = fmt.Errorf("System Owner 1 unavailable, check admin-owner-id in TA config: %w. Restarting the connection ...", err)
			time.Sleep(retry_delay)
			continue
		}

		logger.InitLog.Infof("[TA Client] [Owner ID: %d] [TA ID: %s] Connected to TA successfully. Attempts: %d", taConnector.ownerID, taConnector.taID, i)
		return nil
	}

	if taConnector.conn != nil {
		_ = taConnector.conn.Close()
		taConnector.conn = nil
	}
	taConnector.client = nil
	taConnector.taID = ""

	return fmt.Errorf("Failed to initialise TA-Client after %d attempts: %w", max_retries, lastErr)
}

// Close GRPC connection and empty the variables
func TaClose() error {
	taConnector.mu.Lock()
	defer taConnector.mu.Unlock()

	if taConnector.conn != nil {
		err := taConnector.conn.Close()

		taConnector.conn = nil
		taConnector.client = nil
		taConnector.taID = ""
		return err
	}
	return nil
}
