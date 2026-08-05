package ta

import (
	"testing"

	pb "github.com/free5gc/udr/internal/ta-pb"
)

func TestWriteEventToTA(t *testing.T) {
	t.Log("Initializing TA client connection to localhost:9000...")
	err := TaInit("127.0.0.1:9000")
	if err != nil {
		t.Fatalf("Failed to initialize TA client: %v", err)
	}

	collName := "subscriptionData.provisionedData.amfData"
	key := "imsi-208930000000001_20893"
	value := map[string]interface{}{
		"amfStatus":     "CONNECTED",
		"servingPlmnId": "20893",
		"ueId":          "imsi-208930000000001",
	}

	t.Logf("Triggering TaWrite: coll=%s, key=%s", collName, key)
	err = TaWrite(collName, key, value)
	if err != nil {
		t.Fatalf("TaWrite failed: %v", err)
	}

	t.Log("SUCCESS: Write event successfully committed from UDR client to Trust Anchor DB!")

	// Now read back from Trust Anchor via gRPC
	t.Log("Verifying saved data via gRPC Read call...")
	categoryByte := TaGetCategory(collName)
	req := &pb.Read{
		Location: &pb.Location{
			OwnerId:  ownerID,
			Category: []byte{categoryByte},
			Key:      []byte(key),
		},
		IncludeProof: true,
	}

	res, err := taClient.Read(ctx, req)
	if err != nil {
		t.Fatalf("Read call failed: %v", err)
	}

	t.Logf("Read Success! Stored Value Payload: %s", string(res.Value))
	if res.Proof != nil {
		t.Logf("Merkle Audit Log Leaf Index: %d, Leaf Hash (len): %d bytes", res.Proof.LeafIndex, len(res.Proof.LeafHash))
	}
}
