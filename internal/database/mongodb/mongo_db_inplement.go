package mongodb

import (
	"encoding/json"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/udr/internal/logger"
	"github.com/free5gc/udr/internal/util"
	"github.com/free5gc/udr/pkg/factory"
	"github.com/free5gc/util/mongoapi"

	"github.com/free5gc/udr/internal/ta"
)

type MongoDbConnector struct {
	*factory.Mongodb
}

func NewMongoDbConnector(mongo *factory.Mongodb) MongoDbConnector {
	return MongoDbConnector{
		Mongodb: mongo,
	}
}

func (m MongoDbConnector) PatchDataToDBAndNotify(
	collName string, ueId string, patchItem []models.PatchItem, filter bson.M,
) (origValue, newValue map[string]interface{}, err error) {
	origValue, err = mongoapi.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return
	}

	patchJSON, err := json.Marshal(patchItem)
	if err != nil {
		return
	}

	if err = mongoapi.RestfulAPIJSONPatch(collName, filter, patchJSON); err != nil {
		return
	}

	newValue, err = mongoapi.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return
	}

	// Mirror write to Trust Anchor asynchronously
	key := ta.TaExtractfromFilter(filter)
	go func() {
		if taErr := ta.TaWrite(collName, key, newValue); taErr != nil {
			logger.DataRepoLog.Errorf("Mirror to TA failed: %v", taErr)
		}
	}()

	return
}

func (m MongoDbConnector) GetDataFromDB(
	collName string, filter bson.M) (
	map[string]interface{}, *models.ProblemDetails,
) {
	data, err := mongoapi.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return nil, openapi.ProblemDetailsSystemFailure(err.Error())
	}
	if data == nil {
		return nil, util.ProblemDetailsNotFound("DATA_NOT_FOUND")
	}
	return data, nil
}

func (m MongoDbConnector) GetDataFromDBWithArg(collName string, filter bson.M, strength int) (
	map[string]interface{}, *models.ProblemDetails,
) {
	data, err := mongoapi.RestfulAPIGetOne(collName, filter, strength)
	if err != nil {
		return nil, openapi.ProblemDetailsSystemFailure(err.Error())
	}
	if data == nil {
		logger.ConsumerLog.Errorln("filter: ", filter)
		return nil, util.ProblemDetailsNotFound("DATA_NOT_FOUND")
	}

	return data, nil
}

func (m MongoDbConnector) DeleteDataFromDB(collName string, filter bson.M) {
	if err := mongoapi.RestfulAPIDeleteOne(collName, filter); err != nil {
		logger.DataRepoLog.Errorf("deleteDataFromDB: %+v", err)
	}
}
