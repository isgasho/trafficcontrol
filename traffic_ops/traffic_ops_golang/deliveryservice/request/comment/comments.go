package comment

/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/apache/trafficcontrol/lib/go-log"
	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/apache/trafficcontrol/lib/go-tc/tovalidate"
	"github.com/apache/trafficcontrol/lib/go-util"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/api"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/dbhelpers"

	"github.com/go-ozzo/ozzo-validation"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

//we need a type alias to define functions on
type TODeliveryServiceRequestComment struct {
	ReqInfo *api.APIInfo `json:"-"`
	tc.DeliveryServiceRequestCommentNullable
}

func GetTypeSingleton() api.CRUDFactory {
	return func(reqInfo *api.APIInfo) api.CRUDer {
		toReturn := TODeliveryServiceRequestComment{reqInfo, tc.DeliveryServiceRequestCommentNullable{}}
		return &toReturn
	}
}

func (comment TODeliveryServiceRequestComment) GetKeyFieldsInfo() []api.KeyFieldInfo {
	return []api.KeyFieldInfo{{"id", api.GetIntKey}}
}

//Implementation of the Identifier, Validator interface functions
func (comment TODeliveryServiceRequestComment) GetKeys() (map[string]interface{}, bool) {
	if comment.ID == nil {
		return map[string]interface{}{"id": 0}, false
	}
	return map[string]interface{}{"id": *comment.ID}, true
}

func (comment *TODeliveryServiceRequestComment) SetKeys(keys map[string]interface{}) {
	i, _ := keys["id"].(int) //this utilizes the non panicking type assertion, if the thrown away ok variable is false i will be the zero of the type, 0 here.
	comment.ID = &i
}

func (comment TODeliveryServiceRequestComment) GetAuditName() string {
	if comment.ID != nil {
		return strconv.Itoa(*comment.ID)
	}
	return "unknown"
}

func (comment TODeliveryServiceRequestComment) GetType() string {
	return "deliveryservice_request_comment"
}

func (comment TODeliveryServiceRequestComment) Validate() error {
	errs := validation.Errors{
		"deliveryServiceRequestId": validation.Validate(comment.DeliveryServiceRequestID, validation.NotNil),
		"value":                    validation.Validate(comment.Value, validation.NotNil),
	}
	return util.JoinErrs(tovalidate.ToErrors(errs))
}

func (comment *TODeliveryServiceRequestComment) Create() (error, tc.ApiErrorType) {
	userID := tc.IDNoMod(comment.ReqInfo.User.ID)
	comment.AuthorID = &userID

	resultRows, err := comment.ReqInfo.Tx.NamedQuery(insertQuery(), comment)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			err, eType := dbhelpers.ParsePQUniqueConstraintError(pqErr)
			if eType == tc.DataConflictError {
				return errors.New("a comment with " + err.Error()), eType
			}
			return err, eType
		} else {
			log.Errorf("received non pq error: %++v from create execution", err)
			return tc.DBError, tc.SystemError
		}
	}
	defer resultRows.Close()

	var id int
	var lastUpdated tc.TimeNoMod
	rowsAffected := 0
	for resultRows.Next() {
		rowsAffected++
		if err := resultRows.Scan(&id, &lastUpdated); err != nil {
			log.Error.Printf("could not scan id from insert: %s\n", err)
			return tc.DBError, tc.SystemError
		}
	}
	if rowsAffected == 0 {
		err = errors.New("no cdn was inserted, no id was returned")
		log.Errorln(err)
		return tc.DBError, tc.SystemError
	} else if rowsAffected > 1 {
		err = errors.New("too many ids returned from comment insert")
		log.Errorln(err)
		return tc.DBError, tc.SystemError
	}
	comment.SetKeys(map[string]interface{}{"id": id})
	comment.LastUpdated = &lastUpdated

	return nil, tc.NoError
}

func (comment *TODeliveryServiceRequestComment) Read(parameters map[string]string) ([]interface{}, []error, tc.ApiErrorType) {
	var rows *sqlx.Rows

	// Query Parameters to Database Query column mappings
	// see the fields mapped in the SQL query
	queryParamsToQueryCols := map[string]dbhelpers.WhereColumnInfo{
		"authorId":                 dbhelpers.WhereColumnInfo{"dsrc.author_id", nil},
		"author":                   dbhelpers.WhereColumnInfo{"a.username", nil},
		"deliveryServiceRequestId": dbhelpers.WhereColumnInfo{"dsrc.deliveryservice_request_id", nil},
		"id": dbhelpers.WhereColumnInfo{"dsrc.id", api.IsInt},
	}
	where, orderBy, queryValues, errs := dbhelpers.BuildWhereAndOrderBy(parameters, queryParamsToQueryCols)
	if len(errs) > 0 {
		return nil, errs, tc.DataConflictError
	}

	query := selectQuery() + where + orderBy
	log.Debugln("Query is ", query)

	rows, err := comment.ReqInfo.Tx.NamedQuery(query, queryValues)
	if err != nil {
		log.Errorf("Error querying delivery service request comments: %v", err)
		return nil, []error{tc.DBError}, tc.SystemError
	}
	defer rows.Close()

	comments := []interface{}{}
	for rows.Next() {
		var s tc.DeliveryServiceRequestCommentNullable
		if err = rows.StructScan(&s); err != nil {
			log.Errorf("error parsing delivery service request comment rows: %v", err)
			return nil, []error{tc.DBError}, tc.SystemError
		}
		comments = append(comments, s)
	}

	return comments, []error{}, tc.NoError
}

func (comment *TODeliveryServiceRequestComment) Update() (error, tc.ApiErrorType) {

	var current TODeliveryServiceRequestComment
	err := comment.ReqInfo.Tx.QueryRowx(selectQuery() + `WHERE dsrc.id=` + strconv.Itoa(*comment.ID)).StructScan(&current)
	if err != nil {
		log.Errorf("Error querying DeliveryServiceRequestComments: %v", err)
		return err, tc.SystemError
	}

	userID := tc.IDNoMod(comment.ReqInfo.User.ID)
	if *current.AuthorID != userID {
		return errors.New("Comments can only be updated by the author"), tc.DataConflictError
	}

	log.Debugf("about to run exec query: %s with comment: %++v", updateQuery(), comment)
	resultRows, err := comment.ReqInfo.Tx.NamedQuery(updateQuery(), comment)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			err, eType := dbhelpers.ParsePQUniqueConstraintError(pqErr)
			if eType == tc.DataConflictError {
				return errors.New("a comment with " + err.Error()), eType
			}
			return err, eType
		} else {
			log.Errorf("received error: %++v from update execution", err)
			return tc.DBError, tc.SystemError
		}
	}
	defer resultRows.Close()

	var lastUpdated tc.TimeNoMod
	rowsAffected := 0
	for resultRows.Next() {
		rowsAffected++
		if err := resultRows.Scan(&lastUpdated); err != nil {
			log.Error.Printf("could not scan lastUpdated from insert: %s\n", err)
			return tc.DBError, tc.SystemError
		}
	}
	log.Debugf("lastUpdated: %++v", lastUpdated)
	comment.LastUpdated = &lastUpdated
	if rowsAffected != 1 {
		if rowsAffected < 1 {
			return errors.New("no cdn found with this id"), tc.DataMissingError
		} else {
			return fmt.Errorf("this update affected too many rows: %d", rowsAffected), tc.SystemError
		}
	}
	return nil, tc.NoError
}

func (comment *TODeliveryServiceRequestComment) Delete() (error, tc.ApiErrorType) {

	var current TODeliveryServiceRequestComment
	err := comment.ReqInfo.Tx.QueryRowx(selectQuery() + `WHERE dsrc.id=` + strconv.Itoa(*comment.ID)).StructScan(&current)
	if err != nil {
		log.Errorf("Error querying DeliveryServiceRequestComments: %v", err)
		return err, tc.SystemError
	}

	userID := tc.IDNoMod(comment.ReqInfo.User.ID)
	if *current.AuthorID != userID {
		return errors.New("Comments can only be deleted by the author"), tc.DataConflictError
	}

	log.Debugf("about to run exec query: %s with comment: %++v", deleteQuery(), comment)
	result, err := comment.ReqInfo.Tx.NamedExec(deleteQuery(), comment)
	if err != nil {
		log.Errorf("received error: %++v from delete execution", err)
		return tc.DBError, tc.SystemError
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return tc.DBError, tc.SystemError
	}
	if rowsAffected != 1 {
		if rowsAffected < 1 {
			return errors.New("no comment with that id found"), tc.DataMissingError
		} else {
			return fmt.Errorf("this delete affected too many rows: %d", rowsAffected), tc.SystemError
		}
	}
	return nil, tc.NoError
}

func insertQuery() string {
	query := `INSERT INTO deliveryservice_request_comment (
author_id,
deliveryservice_request_id,
value) VALUES (
:author_id,
:deliveryservice_request_id,
:value) RETURNING id,last_updated`
	return query
}

func selectQuery() string {
	query := `SELECT
a.username AS author,
dsrc.author_id,
dsrc.deliveryservice_request_id,
dsr.deliveryservice->>'xmlId' as xml_id,
dsrc.id,
dsrc.last_updated,
dsrc.value
FROM deliveryservice_request_comment dsrc
JOIN tm_user a ON dsrc.author_id = a.id
JOIN deliveryservice_request dsr ON dsrc.deliveryservice_request_id = dsr.id
`
	return query
}

func updateQuery() string {
	query := `UPDATE
deliveryservice_request_comment SET
deliveryservice_request_id=:deliveryservice_request_id,
value=:value
WHERE id=:id RETURNING last_updated`
	return query
}

func deleteQuery() string {
	query := `DELETE FROM deliveryservice_request_comment
WHERE id=:id`
	return query
}
