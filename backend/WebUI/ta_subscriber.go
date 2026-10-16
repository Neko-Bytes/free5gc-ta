package WebUI

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/free5gc/openapi/models"
	"github.com/free5gc/webconsole/backend/logger"
	"github.com/free5gc/webconsole/backend/ta"
)

// TaSaveSubscriber writes all subscriber data structures to Trust Anchor under the subscriber's owner ID.
func TaSaveSubscriber(ueId string, servingPlmnId string, subsData *SubsData) error {
	if ueId == "" {
		return fmt.Errorf("[TaSaveSubscriber]: empty ueId")
	}

	ownerID, err := ta.TaCreateOwnerID(ueId)
	if err != nil {
		return fmt.Errorf("[TaSaveSubscriber]: failed to allocate Owner ID for %s: %w", ueId, err)
	}

	// 1. Write Web Authentication Subscription (Cat 0x00, key: "webAuth")
	if err := ta.TaWrite(ownerID, ta.CategoryDefault, "webAuth", subsData.WebAuthenticationSubscription); err != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write webAuth: %v", err)
	}

	// 2. Write 3GPP Authentication Subscription (Cat 0x03, key: "auth")
	authSubs, errModel := WebAuthSubToModels(subsData.WebAuthenticationSubscription)
	if errModel != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] WebAuthSubToModels err: %+v", errModel)
	} else if authSubs != nil {
		if err := ta.TaWrite(ownerID, ta.CategorySubscriptionData, "auth", authSubs); err != nil {
			logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write auth: %v", err)
		}
	}

	// 3. Write Access & Mobility Data (Cat 0x03, key: "amData_" + servingPlmnId)
	amKey := "amData_" + servingPlmnId
	if err := ta.TaWrite(ownerID, ta.CategorySubscriptionData, amKey, subsData.AccessAndMobilitySubscriptionData); err != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write amData: %v", err)
	}

	// 4. Write SMF Selection Data (Cat 0x03, key: "smfSelData_" + servingPlmnId)
	smfKey := "smfSelData_" + servingPlmnId
	if err := ta.TaWrite(ownerID, ta.CategorySubscriptionData, smfKey, subsData.SmfSelectionSubscriptionData); err != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write smfSelData: %v", err)
	}

	// 5. Write Session Management Data (Cat 0x03, key: "smData_" + servingPlmnId)
	smKey := "smData_" + servingPlmnId
	if len(subsData.SessionManagementSubscriptionData) > 0 {
		// Store the primary/first SM session data, or entire slice as JSON
		if err := ta.TaWrite(ownerID, ta.CategorySubscriptionData, smKey, subsData.SessionManagementSubscriptionData[0]); err != nil {
			logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write smData: %v", err)
		}
		// Also store full array in Cat 0x00 for complete WebUI reconstruction
		_ = ta.TaWrite(ownerID, ta.CategoryDefault, "all_smData_"+servingPlmnId, subsData.SessionManagementSubscriptionData)
	}

	// 6. Write AM Policy Data (Cat 0x04, key: "amPolicy")
	if err := ta.TaWrite(ownerID, ta.CategoryPolicyData, "amPolicy", subsData.AmPolicyData); err != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write amPolicy: %v", err)
	}

	// 7. Write SM Policy Data (Cat 0x04, key: "smPolicy")
	if err := ta.TaWrite(ownerID, ta.CategoryPolicyData, "smPolicy", subsData.SmPolicyData); err != nil {
		logger.ProcLog.Errorf("[TaSaveSubscriber] failed to write smPolicy: %v", err)
	}

	// 8. Store PLMN reference and auxiliary UI arrays in Cat 0x00
	_ = ta.TaWrite(ownerID, ta.CategoryDefault, "plmnId", servingPlmnId)
	if subsData.FlowRules != nil {
		_ = ta.TaWrite(ownerID, ta.CategoryDefault, "flowRules", subsData.FlowRules)
	}
	if subsData.QosFlows != nil {
		_ = ta.TaWrite(ownerID, ta.CategoryDefault, "qosFlows", subsData.QosFlows)
	}
	if subsData.ChargingDatas != nil {
		_ = ta.TaWrite(ownerID, ta.CategoryDefault, "chargingDatas", subsData.ChargingDatas)
	}

	logger.ProcLog.Infof("[TaSaveSubscriber] Successfully provisioned subscriber %s (Owner ID %d) in Trust Anchor", ueId, ownerID)
	return nil
}

// TaGetSubscriber retrieves subscriber details from Trust Anchor for WebUI
func TaGetSubscriber(ueId string, servingPlmnId string) (*SubsData, error) {
	ownerID, err := ta.TaGetOwnerID(ueId)
	if err != nil {
		return nil, fmt.Errorf("subscriber %s not found: %w", ueId, err)
	}

	subsData := &SubsData{
		PlmnID:                            servingPlmnId,
		UeId:                              ueId,
		SessionManagementSubscriptionData: make([]models.SessionManagementSubscriptionData, 0),
		FlowRules:                         make([]FlowRule, 0),
		QosFlows:                          make([]QosFlow, 0),
		ChargingDatas:                     make([]ChargingData, 0),
	}

	// 1. Read Web Authentication Subscription (Cat 0x00, "webAuth")
	if webAuthBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "webAuth"); err == nil && len(webAuthBytes) > 0 {
		_ = json.Unmarshal(webAuthBytes, &subsData.WebAuthenticationSubscription)
	} else {
		// Fallback to 3GPP auth
		if authBytes, err := ta.TaRead(ownerID, ta.CategorySubscriptionData, "auth"); err == nil && len(authBytes) > 0 {
			var authSub models.AuthenticationSubscription
			if err := json.Unmarshal(authBytes, &authSub); err == nil {
				subsData.WebAuthenticationSubscription = WebAuthenticationSubscription{
					AuthenticationMethod: authSub.AuthenticationMethod,
					PermanentKey: &PermanentKey{
						PermanentKeyValue: authSub.EncPermanentKey,
					},
					Opc: &Opc{
						OpcValue: authSub.EncOpcKey,
					},
					AuthenticationManagementField: authSub.AuthenticationManagementField,
				}
				if authSub.SequenceNumber != nil {
					subsData.WebAuthenticationSubscription.SequenceNumber = authSub.SequenceNumber.Sqn
				}
			}
		}
	}

	// 2. Read Access & Mobility Data (Cat 0x03, "amData_" + servingPlmnId)
	amKey := "amData_" + servingPlmnId
	if amBytes, err := ta.TaRead(ownerID, ta.CategorySubscriptionData, amKey); err == nil && len(amBytes) > 0 {
		_ = json.Unmarshal(amBytes, &subsData.AccessAndMobilitySubscriptionData)
	}

	// 3. Read Session Management Data
	// Try full array first from Cat 0x00
	if allSmBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "all_smData_"+servingPlmnId); err == nil && len(allSmBytes) > 0 {
		_ = json.Unmarshal(allSmBytes, &subsData.SessionManagementSubscriptionData)
	} else {
		// Single record from Cat 0x03
		smKey := "smData_" + servingPlmnId
		if smBytes, err := ta.TaRead(ownerID, ta.CategorySubscriptionData, smKey); err == nil && len(smBytes) > 0 {
			var smData models.SessionManagementSubscriptionData
			if err := json.Unmarshal(smBytes, &smData); err == nil {
				subsData.SessionManagementSubscriptionData = []models.SessionManagementSubscriptionData{smData}
			}
		}
	}

	// 4. Read SMF Selection Data (Cat 0x03, "smfSelData_" + servingPlmnId)
	smfKey := "smfSelData_" + servingPlmnId
	if smfBytes, err := ta.TaRead(ownerID, ta.CategorySubscriptionData, smfKey); err == nil && len(smfBytes) > 0 {
		_ = json.Unmarshal(smfBytes, &subsData.SmfSelectionSubscriptionData)
	}

	// 5. Read AM Policy Data (Cat 0x04, "amPolicy")
	if amPolBytes, err := ta.TaRead(ownerID, ta.CategoryPolicyData, "amPolicy"); err == nil && len(amPolBytes) > 0 {
		_ = json.Unmarshal(amPolBytes, &subsData.AmPolicyData)
	}

	// 6. Read SM Policy Data (Cat 0x04, "smPolicy")
	if smPolBytes, err := ta.TaRead(ownerID, ta.CategoryPolicyData, "smPolicy"); err == nil && len(smPolBytes) > 0 {
		_ = json.Unmarshal(smPolBytes, &subsData.SmPolicyData)
	}

	// 7. Read FlowRules, QosFlows, ChargingDatas from Cat 0x00
	if frBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "flowRules"); err == nil && len(frBytes) > 0 {
		_ = json.Unmarshal(frBytes, &subsData.FlowRules)
	}
	if qfBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "qosFlows"); err == nil && len(qfBytes) > 0 {
		_ = json.Unmarshal(qfBytes, &subsData.QosFlows)
	}
	if cdBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "chargingDatas"); err == nil && len(cdBytes) > 0 {
		_ = json.Unmarshal(cdBytes, &subsData.ChargingDatas)
	}

	// Guarantee non-nil slices for frontend JSON serialization
	if subsData.FlowRules == nil {
		subsData.FlowRules = make([]FlowRule, 0)
	}
	if subsData.QosFlows == nil {
		subsData.QosFlows = make([]QosFlow, 0)
	}
	if subsData.ChargingDatas == nil {
		subsData.ChargingDatas = make([]ChargingData, 0)
	}
	if subsData.SessionManagementSubscriptionData == nil {
		subsData.SessionManagementSubscriptionData = make([]models.SessionManagementSubscriptionData, 0)
	}

	return subsData, nil
}

// TaListAllSubscribersIE queries Owner 1 phonebook and returns the summary list for WebUI
func TaListAllSubscribersIE() ([]SubsListIE, error) {
	subscribers, err := ta.TaListAllSubscribers()
	if err != nil {
		return nil, fmt.Errorf("failed to list subscribers from Trust Anchor: %w", err)
	}

	subsList := make([]SubsListIE, 0, len(subscribers))
	for ueId, ownerID := range subscribers {
		plmnID := "20893" // Default fallback
		gpsi := ""

		// Try to read stored PLMN ID from Category 0x00
		if plmnBytes, err := ta.TaRead(ownerID, ta.CategoryDefault, "plmnId"); err == nil && len(plmnBytes) > 0 {
			_ = json.Unmarshal(plmnBytes, &plmnID)
		}

		// Read Category 0x03 entries to discover actual amData key
		catEntries, err := ta.TaGetCategoryEntries(ownerID, ta.CategorySubscriptionData)
		if err == nil {
			for k, v := range catEntries {
				if strings.HasPrefix(k, "amData_") {
					plmnID = strings.TrimPrefix(k, "amData_")
					var amData models.AccessAndMobilitySubscriptionData
					if err := json.Unmarshal(v, &amData); err == nil {
						if len(amData.Gpsis) > 0 {
							gpsi = amData.Gpsis[0]
						}
					}
					break
				}
			}
		}

		subsList = append(subsList, SubsListIE{
			PlmnID: plmnID,
			UeId:   ueId,
			Gpsi:   gpsi,
		})
	}

	return subsList, nil
}

// TaDeleteSubscriber purges all subscriber data across all categories and unlinks from Owner 1
func TaDeleteSubscriber(ueId string) error {
	return ta.TaDeleteOwner(ueId)
}
