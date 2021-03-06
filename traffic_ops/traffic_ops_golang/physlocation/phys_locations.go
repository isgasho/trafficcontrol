package physlocation

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

	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

//we need a type alias to define functions on
type TOPhysLocation struct {
	ReqInfo *api.APIInfo `json:"-"`
	tc.PhysLocationNullable
}

func GetTypeSingleton() api.CRUDFactory {
	return func(reqInfo *api.APIInfo) api.CRUDer {
		toReturn := TOPhysLocation{reqInfo, tc.PhysLocationNullable{}}
		return &toReturn
	}
}

func (pl TOPhysLocation) GetKeyFieldsInfo() []api.KeyFieldInfo {
	return []api.KeyFieldInfo{{"id", api.GetIntKey}}
}

//Implementation of the Identifier, Validator interface functions
func (pl TOPhysLocation) GetKeys() (map[string]interface{}, bool) {
	if pl.ID == nil {
		return map[string]interface{}{"id": 0}, false
	}
	return map[string]interface{}{"id": *pl.ID}, true
}

func (pl *TOPhysLocation) SetKeys(keys map[string]interface{}) {
	i, _ := keys["id"].(int) //this utilizes the non panicking type assertion, if the thrown away ok variable is false i will be the zero of the type, 0 here.
	pl.ID = &i
}

func (pl *TOPhysLocation) GetAuditName() string {
	if pl.Name != nil {
		return *pl.Name
	}
	if pl.ID != nil {
		return strconv.Itoa(*pl.ID)
	}
	return "unknown"
}

func (pl *TOPhysLocation) GetType() string {
	return "physLocation"
}

func (pl *TOPhysLocation) Validate() error {
	errs := validation.Errors{
		"address":   validation.Validate(pl.Address, validation.Required),
		"city":      validation.Validate(pl.City, validation.Required),
		"name":      validation.Validate(pl.Name, validation.Required),
		"regionId":  validation.Validate(pl.RegionID, validation.Required, validation.Min(0)),
		"shortName": validation.Validate(pl.ShortName, validation.Required),
		"state":     validation.Validate(pl.State, validation.Required),
		"zip":       validation.Validate(pl.Zip, validation.Required),
	}
	if errs != nil {
		return util.JoinErrs(tovalidate.ToErrors(errs))
	}
	return nil
}

func (pl *TOPhysLocation) Read(parameters map[string]string) ([]interface{}, []error, tc.ApiErrorType) {
	var rows *sqlx.Rows

	// Query Parameters to Database Query column mappings
	// see the fields mapped in the SQL query
	queryParamsToQueryCols := map[string]dbhelpers.WhereColumnInfo{
		"name":   dbhelpers.WhereColumnInfo{"pl.name", nil},
		"id":     dbhelpers.WhereColumnInfo{"pl.id", api.IsInt},
		"region": dbhelpers.WhereColumnInfo{"pl.region", api.IsInt},
	}
	where, orderBy, queryValues, errs := dbhelpers.BuildWhereAndOrderBy(parameters, queryParamsToQueryCols)
	if len(errs) > 0 {
		return nil, errs, tc.DataConflictError
	}

	query := selectQuery() + where + orderBy
	log.Debugln("Query is ", query)

	rows, err := pl.ReqInfo.Tx.NamedQuery(query, queryValues)
	if err != nil {
		log.Errorf("Error querying PhysLocations: %v", err)
		return nil, []error{tc.DBError}, tc.SystemError
	}
	defer rows.Close()

	physLocations := []interface{}{}
	for rows.Next() {
		var s tc.PhysLocationNullable
		if err = rows.StructScan(&s); err != nil {
			log.Errorf("error parsing PhysLocation rows: %v", err)
			return nil, []error{tc.DBError}, tc.SystemError
		}
		physLocations = append(physLocations, s)
	}

	return physLocations, []error{}, tc.NoError

}

func selectQuery() string {

	query := `SELECT
pl.address,
pl.city,
pl.comments,
pl.email,
pl.id,
pl.last_updated,
pl.name,
pl.phone,
pl.poc,
r.id as region,
r.name as region_name,
pl.short_name,
pl.state,
pl.zip
FROM phys_location pl
JOIN region r ON pl.region = r.id`

	return query
}

//The TOPhysLocation implementation of the Updater interface
//all implementations of Updater should use transactions and return the proper errorType
//ParsePQUniqueConstraintError is used to determine if a phys_location with conflicting values exists
//if so, it will return an errorType of DataConflict and the type should be appended to the
//generic error message returned
func (pl *TOPhysLocation) Update() (error, tc.ApiErrorType) {
	log.Debugf("about to run exec query: %s with phys_location: %++v", updateQuery(), pl)
	resultRows, err := pl.ReqInfo.Tx.NamedQuery(updateQuery(), pl)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			err, eType := dbhelpers.ParsePQUniqueConstraintError(pqErr)
			if eType == tc.DataConflictError {
				return errors.New("a phys_location with " + err.Error()), eType
			}
			return err, eType
		}
		log.Errorf("received error: %++v from update execution", err)
		return tc.DBError, tc.SystemError
	}
	defer resultRows.Close()

	// get LastUpdated field -- updated by trigger in the db
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
	pl.LastUpdated = &lastUpdated
	if rowsAffected != 1 {
		if rowsAffected < 1 {
			return errors.New("no phys_location found with this id"), tc.DataMissingError
		}
		return fmt.Errorf("this update affected too many rows: %d", rowsAffected), tc.SystemError
	}
	return nil, tc.NoError
}

//The TOPhysLocation implementation of the Creator interface
//all implementations of Creator should use transactions and return the proper errorType
//ParsePQUniqueConstraintError is used to determine if a phys_location with conflicting values exists
//if so, it will return an errorType of DataConflict and the type should be appended to the
//generic error message returned
//The insert sql returns the id and lastUpdated values of the newly inserted phys_location and have
//to be added to the struct
func (pl *TOPhysLocation) Create() (error, tc.ApiErrorType) {
	resultRows, err := pl.ReqInfo.Tx.NamedQuery(insertQuery(), pl)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			err, eType := dbhelpers.ParsePQUniqueConstraintError(pqErr)
			if eType == tc.DataConflictError {
				return errors.New("a phys_location with " + err.Error()), eType
			}
			return err, eType
		}
		log.Errorf("received non pq error: %++v from create execution", err)
		return tc.DBError, tc.SystemError
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
		err = errors.New("no phys_location was inserted, no id was returned")
		log.Errorln(err)
		return tc.DBError, tc.SystemError
	}
	if rowsAffected > 1 {
		err = errors.New("too many ids returned from phys_location insert")
		log.Errorln(err)
		return tc.DBError, tc.SystemError
	}

	pl.SetKeys(map[string]interface{}{"id": id})
	pl.LastUpdated = &lastUpdated

	return nil, tc.NoError
}

//The PhysLocation implementation of the Deleter interface
//all implementations of Deleter should use transactions and return the proper errorType
func (pl *TOPhysLocation) Delete() (error, tc.ApiErrorType) {

	log.Debugf("about to run exec query: %s with phys_location: %++v", deleteQuery(), pl)
	result, err := pl.ReqInfo.Tx.NamedExec(deleteQuery(), pl)
	if err != nil {
		log.Errorf("received error: %++v from delete execution", err)
		return tc.DBError, tc.SystemError
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return tc.DBError, tc.SystemError
	}
	if rowsAffected < 1 {
		return errors.New("no phys_location with that id found"), tc.DataMissingError
	}
	if rowsAffected > 1 {
		return fmt.Errorf("this create affected too many rows: %d", rowsAffected), tc.SystemError
	}

	return nil, tc.NoError
}

func updateQuery() string {
	query := `UPDATE
phys_location SET
address=:address,
city=:city,
comments=:comments,
email=:email,
name=:name,
phone=:phone,
poc=:poc,
region=:region,
short_name=:short_name,
state=:state,
zip=:zip
WHERE id=:id RETURNING last_updated`
	return query
}

func insertQuery() string {
	query := `INSERT INTO phys_location (
address,
city,
comments,
email,
name,
phone,
poc,
region,
short_name,
state,
zip) VALUES (
:address,
:city,
:comments,
:email,
:name,
:phone,
:poc,
:region,
:short_name,
:state,
:zip) RETURNING id,last_updated`
	return query
}

func deleteQuery() string {
	query := `DELETE FROM phys_location
WHERE id=:id`
	return query
}
