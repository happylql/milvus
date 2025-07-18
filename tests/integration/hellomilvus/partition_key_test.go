/*
 * Licensed to the LF AI & Data foundation under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package hellomilvus

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"github.com/milvus-io/milvus/pkg/v2/common"
	"github.com/milvus-io/milvus/pkg/v2/log"
	"github.com/milvus-io/milvus/pkg/v2/util/funcutil"
	"github.com/milvus-io/milvus/pkg/v2/util/metric"
	"github.com/milvus-io/milvus/tests/integration"
)

func (s *HelloMilvusSuite) TestPartitionKey() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.Cluster

	const (
		dim    = 128
		dbName = ""
		rowNum = 1000
	)

	collectionName := "TestPartitionKey" + funcutil.GenRandomStr()
	schema := integration.ConstructSchema(collectionName, dim, false)
	schema.Fields = append(schema.Fields, &schemapb.FieldSchema{
		FieldID:        102,
		Name:           "pid",
		Description:    "",
		DataType:       schemapb.DataType_Int64,
		TypeParams:     nil,
		IndexParams:    nil,
		IsPartitionKey: true,
	})
	marshaledSchema, err := proto.Marshal(schema)
	s.NoError(err)

	createCollectionStatus, err := c.MilvusClient.CreateCollection(ctx, &milvuspb.CreateCollectionRequest{
		DbName:         dbName,
		CollectionName: collectionName,
		Schema:         marshaledSchema,
		ShardsNum:      common.DefaultShardsNum,
	})
	s.NoError(err)
	if createCollectionStatus.GetErrorCode() != commonpb.ErrorCode_Success {
		log.Warn("createCollectionStatus fail reason", zap.String("reason", createCollectionStatus.GetReason()))
	}
	s.Equal(createCollectionStatus.GetErrorCode(), commonpb.ErrorCode_Success)

	{
		pkColumn := integration.NewInt64FieldDataWithStart(integration.Int64Field, rowNum, 0)
		fVecColumn := integration.NewFloatVectorFieldData(integration.FloatVecField, rowNum, dim)
		partitionKeyColumn := integration.NewInt64SameFieldData("pid", rowNum, 1)
		hashKeys := integration.GenerateHashKeys(rowNum)
		insertResult, err := c.MilvusClient.Insert(ctx, &milvuspb.InsertRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			FieldsData:     []*schemapb.FieldData{pkColumn, fVecColumn, partitionKeyColumn},
			HashKeys:       hashKeys,
			NumRows:        uint32(rowNum),
		})
		s.NoError(err)
		s.Equal(insertResult.GetStatus().GetErrorCode(), commonpb.ErrorCode_Success)
	}

	{
		pkColumn := integration.NewInt64FieldDataWithStart(integration.Int64Field, rowNum, rowNum)
		fVecColumn := integration.NewFloatVectorFieldData(integration.FloatVecField, rowNum, dim)
		partitionKeyColumn := integration.NewInt64SameFieldData("pid", rowNum, 10)
		hashKeys := integration.GenerateHashKeys(rowNum)
		insertResult, err := c.MilvusClient.Insert(ctx, &milvuspb.InsertRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			FieldsData:     []*schemapb.FieldData{pkColumn, fVecColumn, partitionKeyColumn},
			HashKeys:       hashKeys,
			NumRows:        uint32(rowNum),
		})
		s.NoError(err)
		s.Equal(insertResult.GetStatus().GetErrorCode(), commonpb.ErrorCode_Success)
	}

	{
		pkColumn := integration.NewInt64FieldDataWithStart(integration.Int64Field, rowNum, rowNum*2)
		fVecColumn := integration.NewFloatVectorFieldData(integration.FloatVecField, rowNum, dim)
		partitionKeyColumn := integration.NewInt64SameFieldData("pid", rowNum, 100)
		hashKeys := integration.GenerateHashKeys(rowNum)
		insertResult, err := c.MilvusClient.Insert(ctx, &milvuspb.InsertRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			FieldsData:     []*schemapb.FieldData{pkColumn, fVecColumn, partitionKeyColumn},
			HashKeys:       hashKeys,
			NumRows:        uint32(rowNum),
		})
		s.NoError(err)
		s.Equal(insertResult.GetStatus().GetErrorCode(), commonpb.ErrorCode_Success)
	}

	flushResp, err := c.MilvusClient.Flush(ctx, &milvuspb.FlushRequest{
		DbName:          dbName,
		CollectionNames: []string{collectionName},
	})
	s.NoError(err)
	segmentIDs, has := flushResp.GetCollSegIDs()[collectionName]
	ids := segmentIDs.GetData()
	s.Require().NotEmpty(segmentIDs)
	s.Require().True(has)
	flushTs, has := flushResp.GetCollFlushTs()[collectionName]
	s.True(has)

	s.WaitForFlush(ctx, ids, flushTs, dbName, collectionName)
	segments, err := c.ShowSegments(collectionName)
	s.NoError(err)
	s.NotEmpty(segments)
	for _, segment := range segments {
		log.Info("ShowSegments result", zap.String("segment", segment.String()))
	}

	// create index
	createIndexStatus, err := c.MilvusClient.CreateIndex(ctx, &milvuspb.CreateIndexRequest{
		CollectionName: collectionName,
		FieldName:      integration.FloatVecField,
		IndexName:      "_default",
		ExtraParams:    integration.ConstructIndexParam(dim, integration.IndexFaissIvfFlat, metric.L2),
	})
	if createIndexStatus.GetErrorCode() != commonpb.ErrorCode_Success {
		log.Warn("createIndexStatus fail reason", zap.String("reason", createIndexStatus.GetReason()))
	}
	s.NoError(err)
	s.Equal(commonpb.ErrorCode_Success, createIndexStatus.GetErrorCode())

	s.WaitForIndexBuilt(ctx, collectionName, integration.FloatVecField)

	// load
	loadStatus, err := c.MilvusClient.LoadCollection(ctx, &milvuspb.LoadCollectionRequest{
		DbName:         dbName,
		CollectionName: collectionName,
	})
	s.NoError(err)
	if loadStatus.GetErrorCode() != commonpb.ErrorCode_Success {
		log.Warn("loadStatus fail reason", zap.String("reason", loadStatus.GetReason()))
	}
	s.Equal(commonpb.ErrorCode_Success, loadStatus.GetErrorCode())
	s.WaitForLoad(ctx, collectionName)

	{
		// search without partition key
		expr := fmt.Sprintf("%s > 0", integration.Int64Field)
		nq := 10
		topk := 10
		roundDecimal := -1

		params := integration.GetSearchParams(integration.IndexFaissIvfFlat, metric.L2)
		searchReq := integration.ConstructSearchRequest("", collectionName, expr,
			integration.FloatVecField, schemapb.DataType_FloatVector, nil, metric.L2, params, nq, dim, topk, roundDecimal)

		searchResult, err := c.MilvusClient.Search(ctx, searchReq)

		if searchResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("searchResult fail reason", zap.String("reason", searchResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, searchResult.GetStatus().GetErrorCode())
	}

	{
		// search with partition key
		expr := fmt.Sprintf("%s > 0 && pid == 1", integration.Int64Field)
		nq := 10
		topk := 10
		roundDecimal := -1

		params := integration.GetSearchParams(integration.IndexFaissIvfFlat, metric.L2)
		searchReq := integration.ConstructSearchRequest("", collectionName, expr,
			integration.FloatVecField, schemapb.DataType_FloatVector, nil, metric.L2, params, nq, dim, topk, roundDecimal)

		searchResult, err := c.MilvusClient.Search(ctx, searchReq)

		if searchResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("searchResult fail reason", zap.String("reason", searchResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, searchResult.GetStatus().GetErrorCode())
	}

	{
		queryResult, err := c.MilvusClient.Query(ctx, &milvuspb.QueryRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			Expr:           "",
			OutputFields:   []string{"count(*)"},
		})
		if queryResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("searchResult fail reason", zap.String("reason", queryResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, queryResult.GetStatus().GetErrorCode())
	}

	{
		queryResult, err := c.MilvusClient.Query(ctx, &milvuspb.QueryRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			Expr:           "pid == 1",
			OutputFields:   []string{"count(*)"},
		})
		if queryResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("searchResult fail reason", zap.String("reason", queryResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, queryResult.GetStatus().GetErrorCode())
	}

	{
		deleteResult, err := c.MilvusClient.Delete(ctx, &milvuspb.DeleteRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			Expr:           integration.Int64Field + " < 1000",
		})
		if deleteResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("deleteResult fail reason", zap.String("reason", deleteResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, deleteResult.GetStatus().GetErrorCode())
	}

	{
		deleteResult, err := c.MilvusClient.Delete(ctx, &milvuspb.DeleteRequest{
			DbName:         dbName,
			CollectionName: collectionName,
			Expr:           integration.Int64Field + " < 2000 && pid == 10",
		})
		if deleteResult.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
			log.Warn("deleteResult fail reason", zap.String("reason", deleteResult.GetStatus().GetReason()))
		}
		s.NoError(err)
		s.Equal(commonpb.ErrorCode_Success, deleteResult.GetStatus().GetErrorCode())
	}
}
