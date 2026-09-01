package ta

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/free5gc/udr/internal/logger"
	pb "github.com/free5gc/udr/internal/ta-pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type TaClient struct {
	conn    *grpc.ClientConn
	client  pb.TrustAnchorClient
	ownerID uint64
	mu      sync.Mutex
	ctx     context.Context

	// Memory cache
	ueOwnerCache   map[string]uint64
	ueOwnerCacheMu sync.RWMutex // Using RWMutex to allow multiple reads.
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
)

var max_retries = 5
var retry_delay = 2 * time.Second

func TaInit(address string) error {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	var lastErr error
	// To avoid duplicate connection
	if ta.client != nil {
		logger.InitLog.Warnln("A TaClient connection already exists!")
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

		ta.conn = c
		// Create a new client in ta
		ta.client = pb.NewTrustAnchorClient(ta.conn)

		// Add the client as owner to ta db
		res, err := ta.client.AddOwner(context.Background(), &pb.AddOwner{})
		if err != nil {
			ta.conn.Close()
			lastErr = fmt.Errorf("Failed to register owner: %w. Restarting the connection ...", err)
			time.Sleep(retry_delay)
			continue
		}

		ta.ownerID = res.OwnerId

		// Package ownerID as metadata
		ownerBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(ownerBytes, ta.ownerID)
		ta.ctx = metadata.AppendToOutgoingContext(context.Background(), "id-bin", string(ownerBytes))

		logger.InitLog.Infof("[TA Client] [Owner ID: %d] Connected to TA successfully. Attempts: %d", ta.ownerID, i)
		return nil
	}

	return fmt.Errorf("Failed to initialise TA-Client after %d attempts: %w", max_retries, lastErr)
}

// Close GRPC connection and empty the variables
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

// TaWrite writes a JSON-serializable value to a specific owner, category, and key in Trust Anchor
func TaWrite(ownerID uint64, category byte, key string, value interface{}) error {
	// Apparently GRPC calls are thread-safe operations. However our taClient variable isn't. To avoid data-races, add mutex lock, copy the global
	// taclient pointer to a local variable so that we can read the address of the ptr safely without bothering the main pointer.
	ta.mu.Lock()
	client := ta.client
	ta.mu.Unlock()

	if client == nil {
		return fmt.Errorf("[TaWrite]: Trust anchor client is not initialized")
	}

	// Serialise value inside JSON structure/map into single line of bytes of char*
	valBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("[TaWrite]: Failed to serialize value to JSON: %w", err)
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

	// GRPC calls are thread-safe by nature. So no need to use mutex to lock the pointer
	_, err = client.Write(ta.ctx, req)
	if err != nil {
		return fmt.Errorf("[TaWrite]: gRPC write to Trust Anchor failed: %w", err)
	}

	return nil
}

// TaWriteByCollName is a convenience helper for legacy callers in free5GC that write by collection name using system owner(UDR)
func TaWriteByCollName(collName string, key string, value interface{}) error {
	category := TaGetCategory(collName)
	return TaWrite(ta.ownerID, category, key, value)
}

func TaRead(ownerID uint64, category byte, keyString string) ([]byte, error) {
	ta.mu.Lock()
	taclient := ta.client
	ta.mu.Unlock()

	if taclient == nil {
		return nil, fmt.Errorf("[TaRead]: taClient not initialised")
	}

	req := &pb.Read{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{category},
			Key:      []byte(keyString),
		},
		IncludeProof: false,
	}

	res, err := taclient.Read(ta.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaRead]: gRPC read from TA failed: %w", err)
	}
	return res.Value, nil
}

// Gets the category that mongoDB uses to know in which category is being used
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

// // Free5GC uses MongoDB which utilises "hierarchy-based storage" to store data. TA uses RocksDB which uses "key-value" based storage.
// // When data goes to MongoDB, it is sent in simple understandable JSON file. However for RocksDB, we need to extract the data from the JSON file
// // such that: Key = "[Owner_Id] + [category] + [Key]", Value = [valBytes]
// // Hence the info required for Key needs to be extracted from JSON file and concatenated into a single byte/string.
// func TaExtractfromFilter(filter bson.M) string {
// 	if filter == nil {
// 		return ""
// 	}
//
// 	if ueId, ok := filter["ueId"].(string); ok {
// 		// Additional sub-keys:
// 		if servingPlmnId, ok := filter["servingPlmnId"].(string); ok {
// 			return fmt.Sprintf("%s_%s", ueId, servingPlmnId)
// 		}
// 		if pduSessionId, ok := filter["pduSessionId"]; ok {
// 			return fmt.Sprintf("%s_%v", ueId, pduSessionId)
// 		}
// 		if limitId, ok := filter["limitId"].(string); ok {
// 			return fmt.Sprintf("%s_%s", ueId, limitId)
// 		}
// 		if usageMonId, ok := filter["usageMonId"].(string); ok {
// 			return fmt.Sprintf("%s_%s", ueId, usageMonId)
// 		}
// 		return ueId
// 	}
//
// 	if influenceId, ok := filter["influenceId"].(string); ok {
// 		return influenceId
// 	}
// 	if sharedDataId, ok := filter["sharedDataId"].(string); ok {
// 		return sharedDataId
// 	}
// 	if applicationId, ok := filter["applicationId"].(string); ok {
// 		return applicationId
// 	}
// 	if bdtReferenceId, ok := filter["bdtReferenceId"].(string); ok {
// 		return bdtReferenceId
// 	}
//
// 	return fmt.Sprintf("%v", filter)
// }

// Unlike TaRead(), this one returns the whole kvpair of a category from the owner space
func TaGetCategoryEntries(ownerID uint64, category byte) (map[string][]byte, error) {
	ta.mu.Lock()
	taclient := ta.client
	ta.mu.Unlock()

	if taclient == nil {
		return nil, fmt.Errorf("[TaCat]: taclient not initialized")
	}

	req := &pb.GetCategory{
		OwnerId:  ownerID,
		Category: []byte{category},
	}

	res, err := taclient.GetCategory(ta.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaCat]: gRPC GetCategory failed: %w", err)
	}

	kvdata := make(map[string][]byte, len(res.Entries))
	for _, entry := range res.Entries {
		kvdata[string(entry.Key)] = entry.Value
	}

	return kvdata, nil
}

// The free5gc already assigns a IEMI subscriber ID for the user. This function helps to locate or create the
// respective OwnerId for the user and also help to map both the subscriberID and OwnerID using a hashmap table
func TaGetOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaGO]: an empty ueID provided") // Owner 1 is reserved for system / Non-UE data
	}

	// Check in memory cache. If ownerID already exists, return the ID
	ta.ueOwnerCacheMu.RLock()
	if id, exists := ta.ueOwnerCache[ueId]; exists {
		ta.ueOwnerCacheMu.RUnlock()
		return id, nil
	}
	ta.ueOwnerCacheMu.RUnlock()

	// Check in TA DB. If ownerID already exists, return the ID
	// Check the link table (Owner 1, CategoryDefault, Key: "ue-map/" + ueId)
	mapData, err := TaRead(1, CategoryDefault, "ue-map/"+ueId)
	if err == nil && len(mapData) > 0 {
		var storedID uint64
		if err := json.Unmarshal(mapData, &storedID); err == nil && storedID > 0 {
			ta.ueOwnerCacheMu.Lock()
			ta.ueOwnerCache[ueId] = storedID
			ta.ueOwnerCacheMu.Unlock()
			return storedID, nil
		}
	}

	return 0, fmt.Errorf("[TaGO]: Owner/Subscriber (%s) not found in the TA", ueId)
}

func TaCreateOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaCO]: empty ueId")
	}

	// If it already exists, don't create a duplicate
	if existingID, err := TaGetOwnerID(ueId); err == nil {
		return existingID, nil
	}

	// Register new Owner in Trust Anchor
	ta.mu.Lock()
	client := ta.client
	ta.mu.Unlock()

	if client == nil {
		return 0, fmt.Errorf("[TaCO]: Trust anchor client is not initialized")
	}

	res, err := client.AddOwner(ta.ctx, &pb.AddOwner{})
	if err != nil {
		return 0, fmt.Errorf("[TaCO]: Failed to create new owner in TA for %s: %w", ueId, err)
	}
	newOwnerID := res.OwnerId

	// Save the link in Trust Anchor under Owner 1
	if err := TaWrite(1, CategoryDefault, "ue-map/"+ueId, newOwnerID); err != nil {
		logger.UtilLog.Warnf("[TaCO]: Failed to persist ue-map in TA for %s: %v", ueId, err)
	}
	// Update RAM cache
	ta.ueOwnerCacheMu.Lock()
	ta.ueOwnerCache[ueId] = newOwnerID
	ta.ueOwnerCacheMu.Unlock()

	logger.UtilLog.Infof("[TaCO] Linked UE %s <---> Owner ID: %d", ueId, newOwnerID)
	return newOwnerID, nil
}

// TaDelete removes a single record from Trust Anchor
func TaDelete(ownerID uint64, category byte, key string) error {
	// Writing an empty/tombstone entry
	return TaWrite(ownerID, category, key, []byte{})
}

// TaDeleteSubscriber completely empties a subscriber's entire owner space
func TaDeleteOwner(ueId string) error {
	ownerID, err := TaGetOwnerID(ueId)
	if err != nil {
		return fmt.Errorf("[TaDO]: User %s doesn't exist", ueId) // User doesn't even exist
	}

	// 1. Wipe all keys
	allCategories := []byte{
		CategoryDefault,
		CategoryInfluenceData,
		CategoryPdf,
		CategorySubscriptionData,
		CategoryPolicyData,
	}
	for _, cat := range allCategories {
		entries, err := TaGetCategoryEntries(ownerID, cat)
		if err == nil {
			for key := range entries {
				_ = TaDelete(ownerID, cat, key) // Wipe every entry
			}
		}
	}

	// 2. Wipe the phonebook link in Owner 1
	_ = TaDelete(1, CategoryDefault, "ue-map/"+ueId)

	// 3. Clean RAM cache
	ta.ueOwnerCacheMu.Lock()
	delete(ta.ueOwnerCache, ueId)
	ta.ueOwnerCacheMu.Unlock()

	logger.UtilLog.Infof("[TaDO] Fully purged subscriber %s (Owner %d)", ueId, ownerID)
	return nil
}
