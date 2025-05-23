// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package pulsar

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/apache/pulsar-client-go/pulsar/backoff"

	"github.com/apache/pulsar-client-go/pulsar/crypto"
	"github.com/apache/pulsar-client-go/pulsaradmin"
	"github.com/apache/pulsar-client-go/pulsaradmin/pkg/admin/config"
	"github.com/apache/pulsar-client-go/pulsaradmin/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestReaderConfigErrors(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	consumer, err := client.CreateReader(ReaderOptions{
		Topic: "my-topic",
	})
	assert.Nil(t, consumer)
	assert.NotNil(t, err)

	consumer, err = client.CreateReader(ReaderOptions{
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, consumer)
	assert.NotNil(t, err)
}

func TestReaderConfigSubscribeName(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	consumer, err := client.CreateReader(ReaderOptions{
		StartMessageID:   EarliestMessageID(),
		Topic:            uuid.New().String(),
		SubscriptionName: uuid.New().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	assert.NotNil(t, consumer)
}

func TestReaderConfigChunk(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	r1, err := client.CreateReader(ReaderOptions{
		Topic:                       "my-topic1",
		StartMessageID:              EarliestMessageID(),
		MaxPendingChunkedMessage:    50,
		ExpireTimeOfIncompleteChunk: 30 * time.Second,
		AutoAckIncompleteChunk:      true,
	})
	assert.Nil(t, err)
	defer r1.Close()

	// verify specified chunk options
	pcOpts := r1.(*reader).c.options
	assert.Equal(t, 50, pcOpts.MaxPendingChunkedMessage)
	assert.Equal(t, 30*time.Second, pcOpts.ExpireTimeOfIncompleteChunk)
	assert.True(t, pcOpts.AutoAckIncompleteChunk)

	r2, err := client.CreateReader(ReaderOptions{
		Topic:          "my-topic2",
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	defer r2.Close()

	// verify default chunk options
	pcOpts = r2.(*reader).c.options
	assert.Equal(t, 100, pcOpts.MaxPendingChunkedMessage)
	assert.Equal(t, time.Minute, pcOpts.ExpireTimeOfIncompleteChunk)
	assert.False(t, pcOpts.AutoAckIncompleteChunk)
}

func TestReader(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create reader
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	defer reader.Close()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic: topic,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	for i := 0; i < 10; i++ {
		_, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
	}

	// receive 10 messages
	for i := 0; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderOnPartitionedTopic(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	assert.Nil(t, createPartitionedTopic(topic, 3))
	ctx := context.Background()
	// create reader
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	defer reader.Close()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic: topic,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	for i := 0; i < 10; i++ {
		_, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
	}

	// receive 10 messages
	for i := 0; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderConnectError(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: "pulsar://invalid-hostname:6650",
	})

	assert.Nil(t, err)

	defer client.Close()

	reader, err := client.CreateReader(ReaderOptions{
		Topic:          "my-topic",
		StartMessageID: LatestMessageID(),
	})

	// Expect error in creating consumer
	assert.Nil(t, reader)
	assert.NotNil(t, err)

	assert.ErrorContains(t, err, "connection error")
}

func TestReaderOnSpecificMessage(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	msgIDs := [10]MessageID{}
	for i := 0; i < 10; i++ {
		msgID, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, msgID)
		msgIDs[i] = msgID
	}

	// create reader on 5th message (not included)
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: msgIDs[4],
	})

	assert.Nil(t, err)
	defer reader.Close()

	// receive the remaining 5 messages
	for i := 5; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}

	// create reader on 5th message (included)
	readerInclusive, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          msgIDs[4],
		StartMessageIDInclusive: true,
	})

	assert.Nil(t, err)
	defer readerInclusive.Close()

	// receive the remaining 6 messages
	for i := 4; i < 10; i++ {
		msg, err := readerInclusive.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderOnSpecificMessageWithBatching(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:                   topic,
		DisableBatching:         false,
		BatchingMaxMessages:     3,
		BatchingMaxPublishDelay: 1 * time.Second,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	msgIDs := [10]MessageID{}
	for i := 0; i < 10; i++ {
		idx := i

		producer.SendAsync(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		}, func(id MessageID, _ *ProducerMessage, err error) {
			assert.NoError(t, err)
			assert.NotNil(t, id)
			msgIDs[idx] = id
		})
	}

	err = producer.FlushWithCtx(context.Background())
	assert.NoError(t, err)

	// create reader on 5th message (not included)
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: msgIDs[4],
	})

	assert.Nil(t, err)
	defer reader.Close()

	// receive the remaining 5 messages
	for i := 5; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}

	// create reader on 5th message (included)
	readerInclusive, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          msgIDs[4],
		StartMessageIDInclusive: true,
	})

	assert.Nil(t, err)
	defer readerInclusive.Close()

	// receive the remaining 6 messages
	for i := 4; i < 10; i++ {
		msg, err := readerInclusive.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderOnLatestWithBatching(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:                   topic,
		DisableBatching:         false,
		BatchingMaxMessages:     4,
		BatchingMaxPublishDelay: 1 * time.Second,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	msgIDs := [10]MessageID{}
	for i := 0; i < 10; i++ {
		idx := i

		producer.SendAsync(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		}, func(id MessageID, _ *ProducerMessage, err error) {
			assert.NoError(t, err)
			assert.NotNil(t, id)
			msgIDs[idx] = id
		})
	}

	err = producer.FlushWithCtx(context.Background())
	assert.NoError(t, err)

	// create reader on 5th message (not included)
	reader, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          LatestMessageID(),
		StartMessageIDInclusive: false,
	})

	assert.Nil(t, err)
	defer reader.Close()

	// Reader should yield no message since it's at the end of the topic
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	msg, err := reader.Next(ctx)
	assert.Error(t, err)
	assert.Nil(t, msg)
	cancel()
}

func TestReaderHasNextAgainstEmptyTopic(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	// create reader on 5th message (not included)
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          "an-empty-topic",
		StartMessageID: EarliestMessageID(),
	})

	assert.Nil(t, err)
	defer reader.Close()

	assert.Equal(t, reader.HasNext(), false)
}

func TestReaderHasNext(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	for i := 0; i < 10; i++ {
		msgID, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, msgID)
	}

	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})

	assert.Nil(t, err)
	defer reader.Close()

	i := 0
	for reader.HasNext() {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())

		i++
	}

	assert.Equal(t, 10, i)
}

type myMessageID struct {
	data []byte
}

func (id *myMessageID) Serialize() []byte {
	return id.data
}

func (id *myMessageID) LedgerID() int64 {
	return id.LedgerID()
}

func (id *myMessageID) EntryID() int64 {
	return id.EntryID()
}

func (id *myMessageID) BatchIdx() int32 {
	return id.BatchIdx()
}

func (id *myMessageID) BatchSize() int32 {
	return id.BatchSize()
}

func (id *myMessageID) PartitionIdx() int32 {
	return id.PartitionIdx()
}

func (id *myMessageID) String() string {
	mid, err := DeserializeMessageID(id.data)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d", mid.LedgerID(), mid.EntryID(), mid.PartitionIdx())
}

func TestReaderOnSpecificMessageWithCustomMessageID(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	msgIDs := [10]MessageID{}
	for i := 0; i < 10; i++ {
		msgID, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, msgID)
		msgIDs[i] = msgID
	}

	// custom start message ID
	myStartMsgID := &myMessageID{
		data: msgIDs[4].Serialize(),
	}

	// attempt to create reader on 5th message (not included)
	var reader Reader
	assert.NotPanics(t, func() {
		reader, err = client.CreateReader(ReaderOptions{
			Topic:          topic,
			StartMessageID: myStartMsgID,
		})
	})

	assert.Nil(t, err)
	defer reader.Close()

	// receive the remaining 5 messages
	for i := 5; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}

	// create reader on 5th message (included)
	readerInclusive, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          myStartMsgID,
		StartMessageIDInclusive: true,
	})

	assert.Nil(t, err)
	defer readerInclusive.Close()

	// receive the remaining 6 messages
	for i := 4; i < 10; i++ {
		msg, err := readerInclusive.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderSeek(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})
	assert.Nil(t, err)
	defer client.Close()

	topicName := newTopicName()
	ctx := context.Background()

	producer, err := client.CreateProducer(ProducerOptions{
		Topic: topicName,
	})
	assert.Nil(t, err)
	defer producer.Close()

	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topicName,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	defer reader.Close()

	const N = 10
	var seekID MessageID
	for i := 0; i < N; i++ {
		id, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.Nil(t, err)

		if i == 4 {
			seekID = id
		}
	}
	err = producer.FlushWithCtx(context.Background())
	assert.NoError(t, err)

	for i := 0; i < N; i++ {
		msg, err := reader.Next(ctx)
		assert.Nil(t, err)
		assert.Equal(t, fmt.Sprintf("hello-%d", i), string(msg.Payload()))
	}

	err = reader.Seek(seekID)
	assert.Nil(t, err)

	readerOfSeek, err := client.CreateReader(ReaderOptions{
		Topic:                   topicName,
		StartMessageID:          seekID,
		StartMessageIDInclusive: true,
	})
	assert.Nil(t, err)

	msg, err := readerOfSeek.Next(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "hello-4", string(msg.Payload()))
}

func TestReaderLatestInclusiveHasNext(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create reader on the last message (inclusive)
	reader0, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          LatestMessageID(),
		StartMessageIDInclusive: true,
	})

	assert.Nil(t, err)
	defer reader0.Close()

	assert.False(t, reader0.HasNext())

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	var lastMsgID MessageID
	for i := 0; i < 10; i++ {
		lastMsgID, err = producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, lastMsgID)
	}

	// create reader on the last message (inclusive)
	reader, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          LatestMessageID(),
		StartMessageIDInclusive: true,
	})

	assert.Nil(t, err)
	defer reader.Close()

	assert.True(t, reader.HasNext())
	msg, err := reader.Next(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, []byte("hello-9"), msg.Payload())
	assert.Equal(t, lastMsgID.Serialize(), msg.ID().Serialize())

	assert.False(t, reader.HasNext())
}

func TestProducerReaderRSAEncryption(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create reader
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
		Decryption: &MessageDecryptionInfo{
			KeyReader: crypto.NewFileKeyReader("crypto/testdata/pub_key_rsa.pem",
				"crypto/testdata/pri_key_rsa.pem"),
			ConsumerCryptoFailureAction: crypto.ConsumerCryptoFailureActionFail,
		},
	})
	assert.Nil(t, err)
	defer reader.Close()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic: topic,
		Encryption: &ProducerEncryptionInfo{
			KeyReader: crypto.NewFileKeyReader("crypto/testdata/pub_key_rsa.pem",
				"crypto/testdata/pri_key_rsa.pem"),
			ProducerCryptoFailureAction: crypto.ProducerCryptoFailureActionFail,
			Keys:                        []string{"client-rsa.pem"},
		},
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 10 messages
	for i := 0; i < 10; i++ {
		_, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
	}

	// receive 10 messages
	for i := 0; i < 10; i++ {
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)

		expectMsg := fmt.Sprintf("hello-%d", i)
		assert.Equal(t, []byte(expectMsg), msg.Payload())
	}
}

func TestReaderWithSchema(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	schema := NewStringSchema(nil)

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:  topic,
		Schema: schema,
	})
	assert.Nil(t, err)
	defer producer.Close()

	value := "hello pulsar"
	_, err = producer.Send(context.Background(), &ProducerMessage{
		Value: value,
	})
	assert.Nil(t, err)

	// create reader
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
		Schema:         schema,
	})
	assert.Nil(t, err)
	defer reader.Close()

	msg, err := reader.Next(context.Background())
	assert.NoError(t, err)

	var res *string
	err = msg.GetSchemaValue(&res)
	assert.Nil(t, err)
	assert.Equal(t, *res, value)
}

func newTestBackoffPolicy(minBackoff, maxBackoff time.Duration) *testBackoffPolicy {
	return &testBackoffPolicy{
		curBackoff: 0,
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
	}
}

type testBackoffPolicy struct {
	curBackoff, minBackoff, maxBackoff time.Duration
	retryTime                          int
}

func (b *testBackoffPolicy) Next() time.Duration {
	// Double the delay each time
	b.curBackoff += b.curBackoff
	if b.curBackoff.Nanoseconds() < b.minBackoff.Nanoseconds() {
		b.curBackoff = b.minBackoff
	} else if b.curBackoff.Nanoseconds() > b.maxBackoff.Nanoseconds() {
		b.curBackoff = b.maxBackoff
	}
	b.retryTime++

	return b.curBackoff
}
func (b *testBackoffPolicy) IsMaxBackoffReached() bool {
	return false
}

func (b *testBackoffPolicy) Reset() {

}

func (b *testBackoffPolicy) IsExpectedIntervalFrom(startTime time.Time) bool {
	// Approximately equal to expected interval
	if time.Since(startTime) < b.curBackoff-time.Second {
		return false
	}
	if time.Since(startTime) > b.curBackoff+time.Second {
		return false
	}
	return true
}

func TestReaderWithBackoffPolicy(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: serviceURL,
	})
	assert.Nil(t, err)
	defer client.Close()

	bo := newTestBackoffPolicy(1*time.Second, 4*time.Second)
	_reader, err := client.CreateReader(ReaderOptions{
		Topic:          "my-topic",
		StartMessageID: LatestMessageID(),
		BackoffPolicyFunc: func() backoff.Policy {
			return bo
		},
	})
	assert.NotNil(t, _reader)
	assert.Nil(t, err)

	partitionConsumerImp := _reader.(*reader).c.consumers[0]
	// 1 s
	startTime := time.Now()
	partitionConsumerImp.reconnectToBroker(nil)
	assert.True(t, bo.IsExpectedIntervalFrom(startTime))

	// 2 s
	startTime = time.Now()
	partitionConsumerImp.reconnectToBroker(nil)
	assert.True(t, bo.IsExpectedIntervalFrom(startTime))

	// 4 s
	startTime = time.Now()
	partitionConsumerImp.reconnectToBroker(nil)
	assert.True(t, bo.IsExpectedIntervalFrom(startTime))

	// 4 s
	startTime = time.Now()
	partitionConsumerImp.reconnectToBroker(nil)
	assert.True(t, bo.IsExpectedIntervalFrom(startTime))
}

func TestReaderGetLastMessageID(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: serviceURL,
	})
	assert.Nil(t, err)
	topic := newTopicName()
	ctx := context.Background()
	schema := NewStringSchema(nil)
	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
		Schema:          schema,
	})
	assert.Nil(t, err)
	defer producer.Close()

	var lastMsgID MessageID
	// send 10 messages
	for i := 0; i < 10; i++ {
		msgID, err := producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, msgID)
		lastMsgID = msgID
	}

	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	getLastMessageID, err := reader.GetLastMessageID()
	if err != nil {
		return
	}

	assert.Equal(t, lastMsgID.LedgerID(), getLastMessageID.LedgerID())
	assert.Equal(t, lastMsgID.EntryID(), getLastMessageID.EntryID())
}

func TestReaderGetLastMessageIDOnMultiTopics(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: serviceURL,
	})
	assert.Nil(t, err)
	topic := newTopicName()
	assert.Nil(t, createPartitionedTopic(topic, 3))

	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	_, err = reader.GetLastMessageID()
	assert.NotNil(t, err)
}

func createPartitionedTopic(topic string, n int) error {
	admin, err := pulsaradmin.NewClient(&config.Config{})
	if err != nil {
		return err
	}

	topicName, err := utils.GetTopicName(topic)
	if err != nil {
		return err
	}
	err = admin.Topics().Create(*topicName, n)
	if err != nil {
		return err
	}
	return nil
}

func TestReaderHasNextFailed(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL: serviceURL,
	})
	assert.Nil(t, err)
	topic := newTopicName()
	r, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)
	r.(*reader).c.consumers[0].state.Store(consumerClosing)
	assert.False(t, r.HasNext())
}

func TestReaderHasNextRetryFailed(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL:              serviceURL,
		OperationTimeout: 2 * time.Second,
	})
	assert.Nil(t, err)
	topic := newTopicName()
	r, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)

	c := make(chan interface{})
	defer close(c)

	// Close the consumer events loop and assign a mock eventsCh
	pc := r.(*reader).c.consumers[0]
	pc.Close()
	pc.state.Store(consumerReady)
	pc.eventsCh = c

	go func() {
		for e := range c {
			req, ok := e.(*getLastMsgIDRequest)
			assert.True(t, ok, "unexpected event type")
			req.err = errors.New("expected error")
			close(req.doneCh)
		}
	}()
	maxTimer := time.NewTimer(8 * time.Second) // Timer to ensure r.HasNext() doesn't block for more than 3s
	startTime := time.Now().UnixMilli()
	done := make(chan bool)
	go func() {
		assert.False(t, r.HasNext())
		done <- true
	}()

	select {
	case <-maxTimer.C:
		t.Fatal("r.HasNext() blocked for more than 3s")
	case <-done:
		now := time.Now().UnixMilli()
		if now-startTime < 1000 {
			t.Fatal("r.HasNext() blocked for less than 1s")
		}
	}

}

func TestReaderNextReturnsOnClosedConsumer(t *testing.T) {
	client, err := NewClient(ClientOptions{
		URL:              serviceURL,
		OperationTimeout: 2 * time.Second,
	})
	assert.NoError(t, err)
	topic := newTopicName()
	reader, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: EarliestMessageID(),
	})
	assert.Nil(t, err)

	reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var e *Error
	_, err = reader.Next(ctx)
	assert.ErrorAs(t, err, &e)
	assert.Equal(t, ConsumerClosed, e.Result())
}

func testReaderSeekByIDWithHasNext(t *testing.T, startMessageID MessageID, startMessageIDInclusive bool) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// send 100 messages
	var lastMsgID MessageID
	for i := 0; i < 10; i++ {
		lastMsgID, err = producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, lastMsgID)
	}

	reader, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          startMessageID,
		StartMessageIDInclusive: startMessageIDInclusive,
	})
	assert.Nil(t, err)
	defer reader.Close()

	// Seek to last message ID
	err = reader.Seek(lastMsgID)
	assert.NoError(t, err)

	if startMessageIDInclusive {
		assert.True(t, reader.HasNext())
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		msg, err := reader.Next(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
		assert.True(t, messageIDCompare(lastMsgID, msg.ID()) == 0)
		cancel()
	} else {
		assert.False(t, reader.HasNext())
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		msg, err := reader.Next(ctx)
		assert.Error(t, err)
		assert.Nil(t, msg)
		cancel()
	}

}

func TestReaderWithSeekByID(t *testing.T) {
	params := []struct {
		messageID               MessageID
		startMessageIDInclusive bool
	}{
		{EarliestMessageID(), false},
		{EarliestMessageID(), true},
		{LatestMessageID(), false},
		{LatestMessageID(), true},
	}

	for _, c := range params {
		t.Run(fmt.Sprintf("TestReaderSeekByID_%v_%v", c.messageID, c.startMessageIDInclusive),
			func(t *testing.T) {
				testReaderSeekByIDWithHasNext(t, c.messageID, c.startMessageIDInclusive)
			})
	}
}

func testReaderSeekByTimeWithHasNext(t *testing.T, startMessageID MessageID) {
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	assert.Nil(t, err)
	defer client.Close()

	topic := newTopicName()
	ctx := context.Background()

	// create producer
	producer, err := client.CreateProducer(ProducerOptions{
		Topic:           topic,
		DisableBatching: true,
	})
	assert.Nil(t, err)
	defer producer.Close()

	// 1. send 10 messages
	var lastMsgID MessageID
	for i := 0; i < 10; i++ {
		lastMsgID, err = producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello-%d", i)),
		})
		assert.NoError(t, err)

		assert.NotNil(t, lastMsgID)
	}

	// 2. create reader
	reader, err := client.CreateReader(ReaderOptions{
		Topic:                   topic,
		StartMessageID:          startMessageID,
		StartMessageIDInclusive: false,
	})
	assert.Nil(t, err)
	defer reader.Close()

	// 3. Seek time to now
	reader.SeekByTime(time.Now())

	// 4. Should not receive msg
	{
		assert.False(t, reader.HasNext())
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		msg, err := reader.Next(timeoutCtx)
		assert.Error(t, err)
		assert.Nil(t, msg)
		cancel()
	}

	// 5. send more 10 messages
	for i := 0; i < 10; i++ {
		lastMsgID, err = producer.Send(ctx, &ProducerMessage{
			Payload: []byte(fmt.Sprintf("hello2-%d", i)),
		})
		assert.NoError(t, err)
		assert.NotNil(t, lastMsgID)
	}

	// 6. Assert these messages are received
	for i := 0; i < 10; i++ {
		assert.True(t, reader.HasNext())
		msg, err := reader.Next(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("hello2-%d", i), string(msg.Payload()))
	}

	// assert not more msg
	{
		assert.False(t, reader.HasNext())
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		msg, err := reader.Next(timeoutCtx)
		assert.Error(t, err)
		assert.Nil(t, msg)
		cancel()
	}
}
func TestReaderWithSeekByTime(t *testing.T) {
	startMessageIDs := []MessageID{EarliestMessageID(), LatestMessageID()}
	for _, startMsgID := range startMessageIDs {
		t.Run(fmt.Sprintf("TestReaderSeekByTime_%v", startMsgID), func(t *testing.T) {
			testReaderSeekByTimeWithHasNext(t, startMsgID)
		})
	}
}

func TestReaderReadFromLatest(t *testing.T) {
	topic := newTopicName()
	client, err := NewClient(ClientOptions{
		URL: lookupURL,
	})

	require.NoError(t, err)
	defer client.Close()

	r, err := client.CreateReader(ReaderOptions{
		Topic:          topic,
		StartMessageID: LatestMessageID(),
	})
	require.NoError(t, err)
	defer r.Close()

	p, err := client.CreateProducer(ProducerOptions{
		Topic: topic,
	})
	require.NoError(t, err)
	defer p.Close()

	// Send messages
	for i := 0; i < 10; i++ {
		msg := &ProducerMessage{
			Key:     "key",
			Payload: []byte(fmt.Sprintf("message-%d", i)),
		}
		id, err := p.Send(context.Background(), msg)
		require.NoError(t, err)
		require.NotNil(t, id)
	}

	// Read and verify messages
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		msg, err := r.Next(ctx)
		cancel()
		require.NoError(t, err)
		require.NotNil(t, msg)

		// Verify message key
		require.Equal(t, "key", msg.Key())

		// Verify message payload
		expectedPayload := fmt.Sprintf("message-%d", i)
		require.Equal(t, []byte(expectedPayload), msg.Payload())
	}

	// Verify no more messages
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	msg, err := r.Next(ctx)
	require.Error(t, err)
	require.Nil(t, msg)
}
