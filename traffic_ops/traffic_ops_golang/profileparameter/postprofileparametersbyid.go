package profileparameter

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
	"net/http"

	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/api"
	dbhelp "github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/dbhelpers"
)

func PostProfileParamsByID(w http.ResponseWriter, r *http.Request) {
	inf, userErr, sysErr, errCode := api.NewInfo(r, []string{"id"}, []string{"id"})
	if userErr != nil || sysErr != nil {
		api.HandleErr(w, r, errCode, userErr, sysErr)
		return
	}
	defer inf.Close()

	profParams := tc.ProfileParametersByNamePost{}
	if err := api.Parse(r.Body, inf.Tx.Tx, &profParams); err != nil {
		api.HandleErr(w, r, http.StatusBadRequest, errors.New("parse error: "+err.Error()), nil)
		return
	}

	profileID := inf.IntParams["id"]
	profileName, profileExists, err := dbhelp.GetProfileNameFromID(profileID, inf.Tx.Tx)
	if err != nil {
		api.HandleErr(w, r, http.StatusInternalServerError, nil, fmt.Errorf("getting profile ID %d: "+err.Error(), profileID))
		return
	}
	if !profileExists {
		api.HandleErr(w, r, http.StatusBadRequest, fmt.Errorf("no profile with ID %d exists", profileID), nil)
		return
	}

	insertedObjs, err := insertParametersForProfile(profileName, profParams, inf.Tx.Tx)
	if err != nil {
		api.HandleErr(w, r, http.StatusInternalServerError, nil, errors.New("posting profile parameters by name: "+err.Error()))
		return
	}

	resp := tc.ProfileParameterPostResp{Parameters: insertedObjs, ProfileName: profileName, ProfileID: profileID}
	*inf.CommitTx = true
	api.WriteRespAlertObj(w, r, tc.SuccessLevel, "Assign parameters successfully to profile "+profileName, resp)
}
