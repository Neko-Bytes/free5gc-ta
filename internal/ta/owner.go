package ta

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/free5gc/udr/internal/logger"
	pb "github.com/free5gc/udr/internal/ta-pb"
)

// TaGetOwnerID resolves the OwnerID for a given subscriber ueId (IMSI/SUPI).
// It checks the in-memory RAM cache first, then falls back to Owner 1 phonebook in Trust Anchor.
func TaGetOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaGO]: an empty ueID provided") // Owner 1 is reserved for system / Non-UE data
	}

	// Check in memory cache. If ownerID already exists, return the ID
	taConnector.ueOwnerCacheMu.RLock()
	if id, exists := taConnector.ueOwnerCache[ueId]; exists {
		taConnector.ueOwnerCacheMu.RUnlock()
		return id, nil
	}
	taConnector.ueOwnerCacheMu.RUnlock()

	// Check in TA DB. If ownerID already exists, return the ID
	// Check the link table (Owner 1, CategoryDefault, Key: "ue-map/" + ueId)
	mapData, err := TaRead(1, CategoryDefault, "ue-map/"+ueId)
	if err == nil && len(mapData) > 0 {
		var storedID uint64
		if err := json.Unmarshal(mapData, &storedID); err == nil && storedID > 0 {
			taConnector.ueOwnerCacheMu.Lock()
			taConnector.ueOwnerCache[ueId] = storedID
			taConnector.ueOwnerCacheMu.Unlock()
			return storedID, nil
		}
	}

	return 0, fmt.Errorf("[TaGO]: Owner/Subscriber (%s) not found in the TA", ueId)
}

// TaCreateOwnerID allocates a new OwnerID in Trust Anchor for a subscriber if one does not already exist,
// registers the mapping in Owner 1 phonebook, and updates the local RAM cache.
func TaCreateOwnerID(ueId string) (uint64, error) {
	if strings.TrimSpace(ueId) == "" {
		return 0, fmt.Errorf("[TaCO]: empty ueId")
	}

	// If it already exists, don't create a duplicate
	if existingID, err := TaGetOwnerID(ueId); err == nil {
		return existingID, nil
	}

	// Register new Owner in Trust Anchor
	taConnector.mu.Lock()
	client := taConnector.client
	taConnector.mu.Unlock()

	if client == nil {
		return 0, fmt.Errorf("[TaCO]: Trust anchor client is not initialized")
	}

	res, err := client.AddOwner(taConnector.ctx, &pb.AddOwner{})
	if err != nil {
		return 0, fmt.Errorf("[TaCO]: Failed to create new owner in TA for %s: %w", ueId, err)
	}
	newOwnerID := res.OwnerId

	// Save the link in Trust Anchor under Owner 1
	if err := TaWrite(1, CategoryDefault, "ue-map/"+ueId, newOwnerID); err != nil {
		logger.UtilLog.Warnf("[TaCO]: Failed to persist ue-map in TA for %s: %v", ueId, err)
	}
	// Update RAM cache
	taConnector.ueOwnerCacheMu.Lock()
	taConnector.ueOwnerCache[ueId] = newOwnerID
	taConnector.ueOwnerCacheMu.Unlock()

	logger.UtilLog.Infof("[TaCO] Linked UE %s <---> Owner ID: %d", ueId, newOwnerID)
	return newOwnerID, nil
}

// TaDeleteOwner completely purges a subscriber's entire owner space across all categories
// and removes the phonebook entry from Owner 1.
func TaDeleteOwner(ueId string) error {
	ownerID, err := TaGetOwnerID(ueId)
	if err != nil {
		return fmt.Errorf("[TaDO]: User %s doesn't exist", ueId) // User doesn't even exist
	}

	// 1. Wipe all keys across all categories
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
	taConnector.ueOwnerCacheMu.Lock()
	delete(taConnector.ueOwnerCache, ueId)
	taConnector.ueOwnerCacheMu.Unlock()

	logger.UtilLog.Infof("[TaDO] Fully purged subscriber %s (Owner %d)", ueId, ownerID)
	return nil
}
