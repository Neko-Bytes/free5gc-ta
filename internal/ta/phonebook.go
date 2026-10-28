package ta

import (
	"encoding/json"
	"fmt"
)

// Phonebook is nothing but a hash-map of Ownerid - ueID for easier lookups.
func phonebookKey(ueId string) string {
	return "ue-map/" + ueId
}

// Encode phonebook value to bytes
func encodePhonebookValue(ownerID uint64) ([]byte, error) {
	return json.Marshal(ownerID)
}

// Decode []bytes back to normal value
func decodePhonebookValue(data []byte) (uint64, error) {
	var id uint64
	if err := json.Unmarshal(data, &id); err != nil {
		return 0, fmt.Errorf("[TA Phone]: Phonebook value is not a number: %w", err)
	}
	if id == 0 {
		return 0, fmt.Errorf("[TA Phone]: Phonebook holds owner 0, which does not exist")
	}
	return id, nil
}

// Finds the Ownerspace ID for the respective subscriber ueId
func phonebookScanner(ueId string) (uint64, error) {
	data, err := TaRead(systemOwnerID, CategoryPhonebook, phonebookKey(ueId))
	if err != nil {
		return 0, fmt.Errorf("[TA Phone]: Trust anchor service is unreachable: %w", err)
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("[TA Phone]: No phonebook entry for %s", ueId)
	}
	return decodePhonebookValue(data)
}

// Writes into the phonebook
func phonebookInsert(ueId string, ownerID uint64) error {
	err := TaWrite(systemOwnerID, CategoryPhonebook, phonebookKey(ueId), ownerID)

	if err != nil {
		return fmt.Errorf("[TA Phone]: New entry for %s inside phonebook failed: %w", ueId, err)
	}
	return nil
}

// Removes entry from the Phonebook
func PhonebookDelete(ueId string) error {
	err := TaDelete(systemOwnerID, CategoryPhonebook, phonebookKey(ueId))

	if err != nil {
		return fmt.Errorf("[TA Phone]: Removal of %s inside Phonebook failed: %w", ueId, err)
	}
	return nil
}
