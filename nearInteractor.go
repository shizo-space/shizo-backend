package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
    b64 "encoding/base64"
)


type AccessKeyListRequestParams struct {
    RequestType   string    `json:"request_type"`
    Finality string `json:"finality"`
    AccountId  string    `json:"account_id"`
}


type ViewNftRequestParams struct {
    RequestType   string 	    `json:"request_type"`
    Finality 	  string 		`json:"finality"`
    AccountId     string    	`json:"account_id"`
	MethodName    string		`json:"method_name"`
	ArgsBase64     string		`json:"args_base64"`
}



type AccessKeyListResponse struct {
    Result struct {
		Keys [] struct {
			PublicKey string	`json:"public_key"`
		} `json:"keys"`
	}    `json:"result"`
}


type ViewNftListResponse struct {
    Result struct {
		Result	[]byte	`json:"result"`
	}     `json:"result"`
}

type RPCRequest struct {
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
	JSONRPC string      `json:"jsonrpc"`
}


type NearInteractor struct {
	RPCNode string
	MasterAccountId string
}

func (nearInteractor *NearInteractor) getAcountPublicKeys(accountId string) []string {
    params := AccessKeyListRequestParams{RequestType: "view_access_key_list", Finality: "final", AccountId: accountId}

    req := RPCRequest{Method: "query", JSONRPC: "2.0", ID: 1, Params: &params}

    jsonData, _ := json.Marshal(&req)

	request, error := http.NewRequest("POST",  nearInteractor.RPCNode, bytes.NewBuffer(jsonData))
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, error := client.Do(request)
	if error != nil {
		println(error)
		return make([]string, 0)
	}
	defer response.Body.Close()
    
	var res AccessKeyListResponse
	json.NewDecoder(response.Body).Decode(&res)

	publicKeys := make([]string, 0)
	for _, k := range res.Result.Keys {
		publicKeys = append(publicKeys, k.PublicKey)
	}

	return publicKeys

}

func (nearInteractor *NearInteractor) getOwnerByTokenId(tokenId string) string {
	ArgsBase64 :=  b64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("{\"token_id\": \"%v\"}", tokenId)))
	params := ViewNftRequestParams{ArgsBase64: ArgsBase64, RequestType: "call_function", Finality: "optimistic", AccountId: nearInteractor.MasterAccountId, MethodName: "nft_token"}

    req := RPCRequest{Method: "query", JSONRPC: "2.0", ID: 1, Params: &params}


    jsonData, _ := json.Marshal(&req)

	request, error := http.NewRequest("POST", nearInteractor.RPCNode, bytes.NewBuffer(jsonData))
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, error := client.Do(request)
	if error != nil {
		panic(error)
	}
	defer response.Body.Close()
    
	var res ViewNftListResponse
	json.NewDecoder(response.Body).Decode(&res)
	var v map[string]interface{}
	json.Unmarshal(res.Result.Result, &v)
	return v["owner_id"].(string)
}