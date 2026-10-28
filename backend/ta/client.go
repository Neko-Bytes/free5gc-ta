package ta

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/free5gc/webconsole/backend/logger"
	pb "github.com/free5gc/webconsole/backend/ta-pb"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type TaClient struct {
	conn           *grpc.ClientConn
	client         pb.TrustAnchorClient
	ownerID        uint64
	taID           string
	mu             sync.Mutex
	ueOwnerCache   map[string]uint64
	ueOwnerCacheMu sync.RWMutex
}

var ta = &TaClient{
	ueOwnerCache: make(map[string]uint64),
}

const (
	CategoryDefault          byte = 0x00
	CategoryInfluenceData    byte = 0x01
	CategoryPdf              byte = 0x02
	CategorySubscriptionData byte = 0x03
	CategoryPolicyData       byte = 0x04
	CategoryPhonebook        byte = 0x10

	// systemOwnerID is the TA-reserved owner space shared with UDR (admin-owner-id: 1).
	systemOwnerID uint64 = 1
)

var max_retries = 30
var retry_delay = 2 * time.Second

// makeOwnerCtx builds the request context for system Owner 1.
func makeOwnerCtx() context.Context {
	ownerBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(ownerBytes, systemOwnerID)
	return metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))
}

// TaInit connects to the Trust Anchor as system Owner 1.
func TaInit(address string) error {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	var lastErr error
	if ta.client != nil {
		logger.InitLog.Warnln("A TaClient connection already exists!")
		return nil
	}

	for i := 1; i <= max_retries; i++ {
		c, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			lastErr = fmt.Errorf("failed to create gRPC connection: %w", err)
			time.Sleep(retry_delay)
			continue
		}

		client := pb.NewTrustAnchorClient(c)

		// Get trust-anchor ID for this owner
		taIDRes, err := client.GetTaId(context.Background(), &pb.Empty{})
		if err != nil || taIDRes.GetTaId() == "" {
			_ = c.Close()
			if err == nil {
				err = fmt.Errorf("empty TA ID returned")
			}
			lastErr = fmt.Errorf("failed to fetch TA ID: %w", err)
			time.Sleep(retry_delay)
			continue
		}

		// Fail fast if system Owner 1 does not exist.
		if _, err := client.GetMap(makeOwnerCtx(), &pb.GetMap{}); err != nil {
			_ = c.Close()
			lastErr = fmt.Errorf("system Owner 1 unavailable, check admin-owner-id in TA config: %w", err)
			time.Sleep(retry_delay)
			continue
		}

		ta.conn = c
		ta.client = client
		ta.ownerID = systemOwnerID
		ta.taID = taIDRes.GetTaId()

		logger.InitLog.Infof("[TA Client] Using system Owner ID: %d (Attempts: %d)", systemOwnerID, i)
		return nil
	}

	if ta.conn != nil {
		_ = ta.conn.Close()
		ta.conn = nil
	}
	ta.client = nil

	return fmt.Errorf("failed to initialise TA-Client after %d attempts: %w", max_retries, lastErr)
}

func TaClose() error {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if ta.conn != nil {
		err := ta.conn.Close()
		ta.conn = nil
		ta.client = nil
		return err
	}
	return nil
}

func TaWrite(ownerID uint64, category byte, key string, value interface{}) error {
	ta.mu.Lock()
	client := ta.client
	taID := ta.taID
	ta.mu.Unlock()

	if client == nil {
		return fmt.Errorf("[TaWrite]: Trust Anchor client is not initialized")
	}

	var valBytes []byte
	switch v := value.(type) {
	case []byte:
		valBytes = v
	case json.RawMessage:
		valBytes = []byte(v)
	default:
		var err error
		valBytes, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("[TaWrite]: failed to serialize value to JSON: %w", err)
		}
	}

	req := &pb.Write{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(key),
			TaId:     taID,
		},
		Value: valBytes,
	}

	ctx := makeOwnerCtx()
	_, err := client.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("[TaWrite]: gRPC write to Trust Anchor failed: %w", err)
	}

	return nil
}

// TaWriteByCollName writes using system owner (Owner 1)
func TaWriteByCollName(collName string, key string, value interface{}) error {
	category := TaGetCategory(collName)
	return TaWrite(1, category, key, value)
}

func TaRead(ownerID uint64, category byte, keyString string) ([]byte, error) {
	ta.mu.Lock()
	client := ta.client
	taID := ta.taID
	ta.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("[TaRead]: Trust Anchor client not initialised")
	}

	req := &pb.Read{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(keyString),
			TaId:     taID,
		},
		IncludeProof: false,
	}

	ctx := makeOwnerCtx()
	res, err := client.Read(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaRead]: gRPC Read failed: %w", err)
	}

	if len(res.Value) == 0 {
		return nil, fmt.Errorf("[TaRead]: data not found")
	}

	return res.Value, nil
}

func TaDelete(ownerID uint64, category byte, keyString string) error {
	ta.mu.Lock()
	client := ta.client
	taID := ta.taID
	ta.mu.Unlock()

	if client == nil {
		return fmt.Errorf("[TaDelete]: Trust Anchor client is not initialized")
	}

	req := &pb.Write{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(keyString),
			TaId:     taID,
		},
		Value: []byte{}, // Tombstone
	}

	ctx := makeOwnerCtx()
	_, err := client.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("[TaDelete]: gRPC write tombstone failed: %w", err)
	}

	return nil
}

func TaGetCategoryEntries(ownerID uint64, category byte) (map[string][]byte, error) {
	ta.mu.Lock()
	client := ta.client
	ta.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("[TaCat]: Trust Anchor client not initialised")
	}

	req := &pb.GetCategory{
		OwnerId:  ownerID,
		Category: []byte{category},
	}

	ctx := makeOwnerCtx()
	res, err := client.GetCategory(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaCat]: gRPC GetCategory failed: %w", err)
	}

	kvdata := make(map[string][]byte, len(res.Entries))
	for _, entry := range res.Entries {
		if len(entry.Value) > 0 { // Skip tombstones
			kvdata[string(entry.Key)] = entry.Value
		}
	}

	return kvdata, nil
}

func TaGetOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaGO]: empty ueID provided")
	}

	ta.ueOwnerCacheMu.RLock()
	if id, exists := ta.ueOwnerCache[ueId]; exists {
		ta.ueOwnerCacheMu.RUnlock()
		return id, nil
	}
	ta.ueOwnerCacheMu.RUnlock()

	// Look in the Owner 1 phonebook.
	storedID, err := phonebookScanner(ueId)
	if err == nil {
		ta.ueOwnerCacheMu.Lock()
		ta.ueOwnerCache[ueId] = storedID
		ta.ueOwnerCacheMu.Unlock()
		return storedID, nil
	}

	return 0, fmt.Errorf("[TaGO]: no ownerID mapped to ueID: %s: %w", ueId, err)
}

func TaCreateOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaCO]: empty ueID provided")
	}

	ta.ueOwnerCacheMu.Lock()
	defer ta.ueOwnerCacheMu.Unlock()

	if id, exists := ta.ueOwnerCache[ueId]; exists {
		return id, nil
	}

	// Read TA directly. Do not call TaGetOwnerID here: it takes
	// the cache read lock, which we already hold as write lock.
	if storedID, err := phonebookScanner(ueId); err == nil {
		ta.ueOwnerCache[ueId] = storedID
		return storedID, nil
	}

	ta.mu.Lock()
	client := ta.client
	ta.mu.Unlock()

	if client == nil {
		return 0, fmt.Errorf("[TaCO]: Trust Anchor client is not initialized")
	}

	res, err := client.AddOwner(context.Background(), &pb.AddOwner{})
	if err != nil {
		return 0, fmt.Errorf("[TaCO]: AddOwner gRPC failed: %w", err)
	}

	newOwnerID := res.OwnerId

	if err := phonebookInsert(ueId, newOwnerID); err != nil {
		return 0, fmt.Errorf("[TaCO]: failed to record ownerID in Owner 1 phonebook: %w", err)
	}

	if err := TaWrite(newOwnerID, CategorySubscriptionData, "subscriber-id", ueId); err != nil {
		logger.ProcLog.Warnf("[TA Client] Failed to store subscriber-id label for %s (Owner ID %d): %v", ueId, newOwnerID, err)
	}

	ta.ueOwnerCache[ueId] = newOwnerID
	logger.ProcLog.Infof("[TA Client] Allocated Owner ID %d for subscriber %s", newOwnerID, ueId)
	return newOwnerID, nil
}

func TaDeleteOwner(ueId string) error {
	ownerID, err := TaGetOwnerID(ueId)
	if err != nil {
		return nil // Not found, nothing to purge
	}

	categories := []byte{
		CategoryDefault,
		CategoryInfluenceData,
		CategoryPdf,
		CategorySubscriptionData,
		CategoryPolicyData,
	}

	for _, cat := range categories {
		entries, err := TaGetCategoryEntries(ownerID, cat)
		if err != nil {
			continue
		}
		for key := range entries {
			_ = TaDelete(ownerID, cat, key)
		}
	}

	// Remove the phonebook entry from Owner 1.
	_ = PhonebookDelete(ueId)

	ta.ueOwnerCacheMu.Lock()
	delete(ta.ueOwnerCache, ueId)
	ta.ueOwnerCacheMu.Unlock()

	logger.ProcLog.Infof("[TA Client] Purged subscriber %s (Owner ID %d)", ueId, ownerID)
	return nil
}

// TaListAllSubscribers returns map of ueId -> ownerID from the Owner 1 phonebook.
func TaListAllSubscribers() (map[string]uint64, error) {
	entries, err := TaGetCategoryEntries(1, CategoryPhonebook)
	if err != nil {
		return nil, err
	}

	result := make(map[string]uint64)
	for key, val := range entries {
		if strings.HasPrefix(key, "ue-map/") {
			ueId := strings.TrimPrefix(key, "ue-map/")
			if ownerID, err := decodePhonebookValue(val); err == nil {
				result[ueId] = ownerID
			}
		}
	}
	return result, nil
}

func TaGetCategory(collName string) byte {
	switch {
	case collName == "applicationData.influenceData":
		return CategoryInfluenceData
	case collName == "applicationData.pfds":
		return CategoryPdf
	case strings.HasPrefix(collName, "policyData."):
		return CategoryPolicyData
	case strings.HasPrefix(collName, "subscriptionData."):
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
