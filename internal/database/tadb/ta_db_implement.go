package tadb

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/udr/internal/logger"
	"github.com/free5gc/udr/internal/ta"
	"github.com/free5gc/udr/internal/util"
)

// TaDbConnector implements the DbConnector interface using Trust Anchor as the backend database.
type TaDbConnector struct{}

func NewTaDbConnector() TaDbConnector {
	return TaDbConnector{}
}

// GetDataFromDB reads a single record from Trust Anchor
func (t TaDbConnector) GetDataFromDB(
	collName string, filter bson.M,
) (map[string]interface{}, *models.ProblemDetails) {
	category := ta.TaGetCategory(collName)

	ownerID, key, err := resolveOwnerAndKey(collName, filter)
	if err != nil {
		return nil, util.ProblemDetailsNotFound("USER_NOT_FOUND")
	}

	dataBytes, err := ta.TaRead(ownerID, category, key)
	if err != nil || len(dataBytes) == 0 {
		return nil, util.ProblemDetailsNotFound("DATA_NOT_FOUND")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return nil, openapi.ProblemDetailsSystemFailure(err.Error())
	}
	return result, nil
}

// GetDataFromDBWithArg reads a single record (strength arg is MongoDB-specific, ignored for TA)
func (t TaDbConnector) GetDataFromDBWithArg(
	collName string, filter bson.M, strength int,
) (map[string]interface{}, *models.ProblemDetails) {
	return t.GetDataFromDB(collName, filter)
}

// GetManyFromDB reads all records in an owner's category
func (t TaDbConnector) GetManyFromDB(
	collName string, filter bson.M,
) ([]map[string]interface{}, error) {
	category := ta.TaGetCategory(collName)

	ownerID, err := resolveOwnerID(filter)
	if err != nil {
		return nil, err
	}

	entries, err := ta.TaGetCategoryEntries(ownerID, category)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0, len(entries))
	for _, valBytes := range entries {
		if len(valBytes) == 0 {
			continue // Skip tombstones
		}
		var record map[string]interface{}
		if err := json.Unmarshal(valBytes, &record); err == nil {
			results = append(results, record)
		}
	}
	return results, nil
}

// PutDataToDB writes a record to Trust Anchor, creating the owner if needed
func (t TaDbConnector) PutDataToDB(
	collName string, filter bson.M, data map[string]interface{},
) (bool, error) {
	category := ta.TaGetCategory(collName)

	ownerID, key, err := resolveOwnerAndKeyForWrite(collName, filter)
	if err != nil {
		return false, err
	}

	if err := ta.TaWrite(ownerID, category, key, data); err != nil {
		return false, err
	}
	return true, nil
}

// PatchDataToDBAndNotify reads, patches, and writes back a record in Trust Anchor
func (t TaDbConnector) PatchDataToDBAndNotify(
	collName string, ueId string, patchItem []models.PatchItem, filter bson.M,
) (origValue, newValue map[string]interface{}, err error) {
	category := ta.TaGetCategory(collName)

	ownerID, key, err := resolveOwnerAndKey(collName, filter)
	if err != nil {
		return nil, nil, err
	}

	// Read current value
	dataBytes, err := ta.TaRead(ownerID, category, key)
	if err != nil {
		return nil, nil, err
	}

	if len(dataBytes) > 0 {
		if err = json.Unmarshal(dataBytes, &origValue); err != nil {
			return
		}
	} else {
		origValue = make(map[string]interface{})
	}

	// Apply JSON patch
	patchJSON, err := json.Marshal(patchItem)
	if err != nil {
		return
	}

	patchedBytes, err := applyJSONPatch(dataBytes, patchJSON)
	if err != nil {
		return
	}

	// Write patched value back
	if err = ta.TaWrite(ownerID, category, key, json.RawMessage(patchedBytes)); err != nil {
		return
	}

	if err = json.Unmarshal(patchedBytes, &newValue); err != nil {
		return
	}
	return
}

// DeleteDataFromDB removes a record from Trust Anchor
func (t TaDbConnector) DeleteDataFromDB(collName string, filter bson.M) {
	category := ta.TaGetCategory(collName)

	ownerID, key, err := resolveOwnerAndKey(collName, filter)
	if err != nil {
		logger.DataRepoLog.Errorf("[TaDbConnector] DeleteDataFromDB resolve error: %v", err)
		return
	}
	if err := ta.TaDelete(ownerID, category, key); err != nil {
		logger.DataRepoLog.Errorf("[TaDbConnector] DeleteDataFromDB error: %v", err)
	}
}

// DeleteOwnerFromDB completely purges a subscriber's data from Trust Anchor
func (t TaDbConnector) DeleteOwnerFromDB(ueId string) error {
	return ta.TaDeleteOwner(ueId)
}

// --- Private helpers ---

func resolveOwnerID(filter bson.M) (uint64, error) {
	if filter == nil {
		return 1, nil
	}
	if ueId, ok := filter["ueId"].(string); ok && ueId != "" {
		return ta.TaGetOwnerID(ueId)
	}
	return 1, nil
}

func resolveOwnerAndKey(collName string, filter bson.M) (uint64, string, error) {
	ownerID, err := resolveOwnerID(filter)
	if err != nil {
		return 0, "", err
	}
	key := buildKey(collName, filter)
	return ownerID, key, nil
}

func resolveOwnerAndKeyForWrite(collName string, filter bson.M) (uint64, string, error) {
	if filter != nil {
		if ueId, ok := filter["ueId"].(string); ok && ueId != "" {
			ownerID, err := ta.TaCreateOwnerID(ueId)
			if err != nil {
				return 0, "", err
			}
			return ownerID, buildKey(collName, filter), nil
		}
	}
	return 1, buildKey(collName, filter), nil
}

func buildKey(collName string, filter bson.M) string {
	if filter != nil {
		if servingPlmnId, ok := filter["servingPlmnId"].(string); ok {
			return "amData_" + servingPlmnId
		}
		if pduSessionId, ok := filter["pduSessionId"]; ok {
			return fmt.Sprintf("smfReg_%v", pduSessionId)
		}
		if limitId, ok := filter["limitId"].(string); ok {
			return "limit_" + limitId
		}
		if usageMonId, ok := filter["usageMonId"].(string); ok {
			return "usageMon_" + usageMonId
		}
		if influenceId, ok := filter["influenceId"].(string); ok {
			return influenceId
		}
		if applicationId, ok := filter["applicationId"].(string); ok {
			return applicationId
		}
		if bdtReferenceId, ok := filter["bdtReferenceId"].(string); ok {
			return bdtReferenceId
		}
	}

	switch {
	case strings.Contains(collName, "authenticationSubscription"):
		return "auth"
	case strings.Contains(collName, "authenticationStatus"):
		return "authStatus"
	case strings.Contains(collName, "sorData"):
		return "sorData"
	case strings.Contains(collName, "amf3gppAccessRegistration"):
		return "context_amf3gpp"
	case strings.Contains(collName, "amfNon3gppAccessRegistration"):
		return "context_amfNon3gpp"
	case strings.Contains(collName, "smfRegistrations"):
		return "context_smf"
	case strings.Contains(collName, "smsfNon3gpp"):
		return "context_smsfNon3gpp"
	case strings.Contains(collName, "smsf3gpp"):
		return "context_smsf3gpp"
	default:
		return collName
	}
}

func applyJSONPatch(original []byte, patchJSON []byte) ([]byte, error) {
	var doc map[string]interface{}
	if len(original) > 0 {
		if err := json.Unmarshal(original, &doc); err != nil {
			return nil, err
		}
	} else {
		doc = make(map[string]interface{})
	}

	var patches []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(patchJSON, &patches); err != nil {
		return nil, err
	}

	for _, patch := range patches {
		parts := strings.Split(strings.TrimPrefix(patch.Path, "/"), "/")
		switch patch.Op {
		case "add", "replace":
			var val interface{}
			if err := json.Unmarshal(patch.Value, &val); err != nil {
				return nil, err
			}
			setNestedKey(doc, parts, val)
		case "remove":
			removeNestedKey(doc, parts)
		}
	}

	return json.Marshal(doc)
}

func setNestedKey(doc map[string]interface{}, parts []string, val interface{}) {
	if len(parts) == 1 {
		doc[parts[0]] = val
		return
	}
	if sub, ok := doc[parts[0]].(map[string]interface{}); ok {
		setNestedKey(sub, parts[1:], val)
	} else {
		sub = make(map[string]interface{})
		doc[parts[0]] = sub
		setNestedKey(sub, parts[1:], val)
	}
}

func removeNestedKey(doc map[string]interface{}, parts []string) {
	if len(parts) == 1 {
		delete(doc, parts[0])
		return
	}
	if sub, ok := doc[parts[0]].(map[string]interface{}); ok {
		removeNestedKey(sub, parts[1:])
	}
}
