// Copyright 2024 The Tektite Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package load

import (
	json2 "encoding/json"
	"fmt"
	"github.com/spirit-labs/tektite/errors"
	"github.com/spirit-labs/tektite/kafka"
	"math/rand"
	"time"
)

type simpleGenerator struct {
	uniqueIDsPerPartition int64
}

func (s *simpleGenerator) Init() {
}

func (s *simpleGenerator) GenerateMessage(partitionID int32, offset int64, _ *rand.Rand) (*kafka.Message, error) {
	m := make(map[string]interface{})
	customerToken := fmt.Sprintf("customer-token-%d-%d", partitionID, offset%s.uniqueIDsPerPartition)
	m["primary_key_col"] = customerToken
	m["varchar_col"] = fmt.Sprintf("customer-full-name-%s", customerToken)
	m["bigint_col"] = offset % 1000

	json, err := json2.Marshal(&m)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	msg := &kafka.Message{
		Key:       []byte(customerToken),
		Value:     json,
		TimeStamp: time.Now(),
		PartInfo: kafka.PartInfo{
			PartitionID: partitionID,
			Offset:      offset,
		},
	}

	return msg, nil
}

func (s *simpleGenerator) Name() string {
	return "simple"
}

type paymentsGenerator struct {
	uniqueIDsPerPartition int64
	paymentTypes          []string
	currencies            []string
}

func (p *paymentsGenerator) Init() {
	p.paymentTypes = []string{"btc", "p2p", "other"}
	p.currencies = []string{"gbp", "usd", "eur", "aud"}
}

func (p *paymentsGenerator) GenerateMessage(partitionID int32, offset int64, rnd *rand.Rand) (*kafka.Message, error) {
	m := make(map[string]interface{})
	// Payment id must be globally unique - so we include partition id and offset in it
	paymentID := fmt.Sprintf("payment-%010d-%019d", partitionID, offset)
	customerID := fmt.Sprintf("customer-token-%010d-%019d", partitionID, offset%p.uniqueIDsPerPartition)
	m["customer_token"] = customerID
	m["amount"] = fmt.Sprintf("%.2f", float64(rnd.Int31n(1000000))/10)
	m["payment_type"] = p.paymentTypes[int(offset)%len(p.paymentTypes)]
	m["currency"] = p.currencies[int(offset)%len(p.currencies)]
	json, err := json2.Marshal(&m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	msg := &kafka.Message{
		Key:       []byte(paymentID),
		Value:     json,
		TimeStamp: time.Now(),
		PartInfo: kafka.PartInfo{
			PartitionID: partitionID,
			Offset:      offset,
		},
	}
	return msg, nil
}

func (p *paymentsGenerator) Name() string {
	return "payments"
}
