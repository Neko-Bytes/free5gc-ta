package processor

import (
	"github.com/free5gc/udr/internal/database"
	"github.com/free5gc/udr/internal/logger"
	"github.com/free5gc/udr/internal/ta"
	"github.com/free5gc/udr/pkg/app"
	"go.mongodb.org/mongo-driver/bson"
)

type Processor struct {
	app.App
	database.DbConnector
}

func NewProcessor(udr app.App) *Processor {
	return &Processor{
		App:         udr,
		DbConnector: database.NewDbConnector(udr.Config().Configuration.DbConnectorType),
	}
}

// TaWriteMirror extracts the key from a MongoDB filter and mirrors the value to the Trust Anchor in the background.
func (p *Processor) TaWriteMirror(collName string, filter bson.M, value interface{}) {
	key := ta.TaExtractfromFilter(filter)
	go func() {
		if taErr := ta.TaWriteByCollName(collName, key, value); taErr != nil {
			logger.DataRepoLog.Errorf("Mirror to TA failed: %v", taErr)
		}
	}()
}
