package ta

import (
	"encoding/json"
	"fmt"

	pb "github.com/free5gc/udr/internal/ta-pb"
)

// TaWrite writes a JSON-serializable value to a specific owner, category, and key in Trust Anchor
func TaWrite(ownerID uint64, category byte, key string, value interface{}) error {
	// Apparently GRPC calls are thread-safe operations. However our taClient variable isn't. To avoid data-races, add mutex lock, copy the global
	// taclient pointer to a local variable so that we can read the address of the ptr safely without bothering the main pointer.
	taConnector.mu.Lock()
	client := taConnector.client
	taConnector.mu.Unlock()

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
	_, err = client.Write(taConnector.ctx, req)
	if err != nil {
		return fmt.Errorf("[TaWrite]: gRPC write to Trust Anchor failed: %w", err)
	}

	return nil
}

// TaWriteByCollName is a convenience helper for legacy callers in free5GC that write by collection name using system owner(UDR)
func TaWriteByCollName(collName string, key string, value interface{}) error {
	category := TaGetCategory(collName)
	return TaWrite(taConnector.ownerID, category, key, value)
}

// TaRead retrieves data from a specific owner, category, and key in Trust Anchor
func TaRead(ownerID uint64, category byte, keyString string) ([]byte, error) {
	taConnector.mu.Lock()
	taclient := taConnector.client
	taConnector.mu.Unlock()

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

	res, err := taclient.Read(taConnector.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaRead]: gRPC read from TA failed: %w", err)
	}
	return res.Value, nil
}

// TaDelete removes a single record from Trust Anchor (by writing an empty tombstone)
func TaDelete(ownerID uint64, category byte, key string) error {
	return TaWrite(ownerID, category, key, []byte{})
}

// TaGetCategoryEntries returns all key-value pairs of a specific category from the owner space
func TaGetCategoryEntries(ownerID uint64, category byte) (map[string][]byte, error) {
	taConnector.mu.Lock()
	taclient := taConnector.client
	taConnector.mu.Unlock()

	if taclient == nil {
		return nil, fmt.Errorf("[TaCat]: taclient not initialized")
	}

	req := &pb.GetCategory{
		OwnerId:  ownerID,
		Category: []byte{category},
	}

	res, err := taclient.GetCategory(taConnector.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("[TaCat]: gRPC GetCategory failed: %w", err)
	}

	kvdata := make(map[string][]byte, len(res.Entries))
	for _, entry := range res.Entries {
		kvdata[string(entry.Key)] = entry.Value
	}

	return kvdata, nil
}

// TaGetCategory maps mongoDB collection names to Trust Anchor category byte codes
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
