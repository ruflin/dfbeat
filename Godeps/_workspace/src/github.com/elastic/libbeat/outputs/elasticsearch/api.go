package elasticsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/elastic/libbeat/logp"
)

type Elasticsearch struct {
	MaxRetries     int
	connectionPool ConnectionPool
	client         *http.Client
}

type QueryResult struct {
	Ok      bool            `json:"ok"`
	Index   string          `json:"_index"`
	Type    string          `json:"_type"`
	Id      string          `json:"_id"`
	Source  json.RawMessage `json:"_source"`
	Version int             `json:"_version"`
	Found   bool            `json:"found"`
	Exists  bool            `json:"exists"`
	Created bool            `json:"created"`
	Matches []string        `json:"matches"`
}

type SearchResults struct {
	Took   int                        `json:"took"`
	Shards json.RawMessage            `json:"_shards"`
	Hits   Hits                       `json:"hits"`
	Aggs   map[string]json.RawMessage `json:"aggregations"`
}

type Hits struct {
	Total int
	Hits  []json.RawMessage `json:"hits"`
}

func (r QueryResult) String() string {
	out, err := json.Marshal(r)
	if err != nil {
		return "ERROR"
	}
	return string(out)
}

const (
	default_max_retries = 3
)

// Create a connection to Elasticsearch
func NewElasticsearch(urls []string, username string, password string) *Elasticsearch {

	var connection_pool ConnectionPool
	connection_pool.SetConnections(urls, username, password)

	es := Elasticsearch{
		connectionPool: connection_pool,
		client:         &http.Client{},
		MaxRetries:     default_max_retries,
	}
	return &es
}

// Encode parameters in url
func UrlEncode(params map[string]string) string {
	var values url.Values = url.Values{}

	for key, val := range params {
		values.Add(key, string(val))
	}
	return values.Encode()
}

// Create path out of index, doc_type and id that is used for querying Elasticsearch
func MakePath(index string, doc_type string, id string) (string, error) {

	var path string
	if len(doc_type) > 0 {
		if len(id) > 0 {
			path = fmt.Sprintf("/%s/%s/%s", index, doc_type, id)
		} else {
			path = fmt.Sprintf("/%s/%s", index, doc_type)
		}
	} else {
		if len(id) > 0 {
			if len(index) > 0 {
				path = fmt.Sprintf("/%s/%s", index, id)
			} else {
				path = fmt.Sprintf("/%s", id)
			}
		} else {
			path = fmt.Sprintf("/%s", index)
		}
	}
	return path, nil
}

func ReadQueryResult(obj []byte) (*QueryResult, error) {

	var result QueryResult
	if obj == nil {
		return nil, nil
	}
	err := json.Unmarshal(obj, &result)
	if err != nil {
		return nil, err
	}
	return &result, err
}

func ReadSearchResult(obj []byte) (*SearchResults, error) {

	var result SearchResults
	if obj == nil {
		return nil, nil
	}
	err := json.Unmarshal(obj, &result)
	if err != nil {
		return nil, err
	}
	return &result, err
}

func (es *Elasticsearch) SetMaxRetries(max_retries int) {
	es.MaxRetries = max_retries
}

func isConnTimeout(err error) bool {
	return strings.Contains(err.Error(), "i/o timeout")
}

func isConnRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}

// Perform the actual request. If the operation was successful, mark it as live and return the response.
// Mark the Elasticsearch node as dead for a period of time in the case the http request fails with Connection
// Timeout, Connection Refused or returns one of the 503,504 Error Replies.
// It returns the response, if it should retry sending the request and the error
func (es *Elasticsearch) PerformRequest(conn *Connection, req *http.Request) ([]byte, bool, error) {

	req.Header.Add("Accept", "application/json")
	if conn.Username != "" || conn.Password != "" {
		req.SetBasicAuth(conn.Username, conn.Password)
	}

	resp, err := es.client.Do(req)
	if err != nil {
		// request fails
		do_retry := false
		if isConnTimeout(err) || isConnRefused(err) {
			es.connectionPool.MarkDead(conn)
			do_retry = true
		}
		return nil, do_retry, fmt.Errorf("Sending the request fails: %s", err)
	}

	if resp.StatusCode > 299 {
		// request fails
		do_retry := false
		if resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout {
			// status code in {503, 504}
			es.connectionPool.MarkDead(conn)
			do_retry = true
		}
		return nil, do_retry, fmt.Errorf("%v", resp.Status)
	}

	defer resp.Body.Close()
	obj, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		es.connectionPool.MarkDead(conn)
		return nil, true, fmt.Errorf("Reading the response fails: %s", err)
	}

	// request with success
	es.connectionPool.MarkLive(conn)

	return obj, false, nil

}

// Create an HTTP request and send it to Elasticsearch. The request is retransmitted max_retries
// before returning an error.
func (es *Elasticsearch) Request(method string, path string,
	params map[string]string, body interface{}) ([]byte, error) {

	var errors []error

	for attempt := 0; attempt < es.MaxRetries; attempt++ {

		conn := es.connectionPool.GetConnection()
		logp.Debug("elasticsearch", "Use connection %s", conn.Url)

		url := conn.Url + path
		if len(params) > 0 {
			url = url + "?" + UrlEncode(params)
		}

		logp.Debug("elasticsearch", "%s %s %s", method, url, body)

		var obj []byte
		var err error
		if body != nil {
			obj, err = json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("Fail to JSON encode the body: %s", err)
			}
		} else {
			obj = nil
		}
		req, err := http.NewRequest(method, url, bytes.NewReader(obj))
		if err != nil {
			return nil, fmt.Errorf("NewRequest fails: %s", err)
		}

		resp, retry, err := es.PerformRequest(conn, req)
		if retry == true {
			// retry
			if err != nil {
				errors = append(errors, err)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil

	}

	logp.Warn("Request fails to be send after %d retries", es.MaxRetries)

	return nil, fmt.Errorf("Request fails after %d retries. Errors: %v", es.MaxRetries, errors)
}

// Index adds or updates a typed JSON document in a specified index, making it
// searchable. In case id is empty, a new id is created over a HTTP POST request.
// Otherwise, a HTTP PUT request is issued.
// Implements: http://www.elastic.co/guide/en/elasticsearch/reference/current/docs-index_.html
func (es *Elasticsearch) Index(index string, doc_type string, id string,
	params map[string]string, body interface{}) (*QueryResult, error) {

	var method string

	path, err := MakePath(index, doc_type, id)
	if err != nil {
		return nil, fmt.Errorf("MakePath fails: %s", err)
	}
	if len(id) == 0 {
		method = "POST"
	} else {
		method = "PUT"
	}
	resp, err := es.Request(method, path, params, body)
	if err != nil {
		return nil, err
	}
	return ReadQueryResult(resp)
}

// Refresh an index. Call this after doing inserts or creating/deleting
// indexes in unit tests.
func (es *Elasticsearch) Refresh(index string) (*QueryResult, error) {
	path, err := MakePath(index, "", "_refresh")
	if err != nil {
		return nil, err
	}
	resp, err := es.Request("POST", path, nil, nil)
	if err != nil {
		return nil, err
	}

	return ReadQueryResult(resp)
}

// Creates a new index, optionally with settings and mappings passed in
// the body.
// Implements: https://www.elastic.co/guide/en/elasticsearch/reference/current/indices-create-index.html
//
func (es *Elasticsearch) CreateIndex(index string, body interface{}) (*QueryResult, error) {

	path, err := MakePath(index, "", "")
	if err != nil {
		return nil, err
	}

	resp, err := es.Request("PUT", path, nil, body)
	if err != nil {
		return nil, err
	}

	return ReadQueryResult(resp)
}

// Deletes a typed JSON document from a specific index based on its id.
// Implements: http://www.elastic.co/guide/en/elasticsearch/reference/current/docs-delete.html
func (es *Elasticsearch) Delete(index string, doc_type string, id string, params map[string]string) (*QueryResult, error) {

	path, err := MakePath(index, doc_type, id)
	if err != nil {
		return nil, err
	}

	resp, err := es.Request("DELETE", path, params, nil)
	if err != nil {
		return nil, err
	}

	return ReadQueryResult(resp)
}

// A search request can be executed purely using a URI by providing request parameters.
// Implements: http://www.elastic.co/guide/en/elasticsearch/reference/current/search-uri-request.html
func (es *Elasticsearch) SearchUri(index string, doc_type string, params map[string]string) (*SearchResults, error) {

	path, err := MakePath(index, doc_type, "_search")
	if err != nil {
		return nil, err
	}

	resp, err := es.Request("GET", path, params, nil)
	if err != nil {
		return nil, err
	}
	return ReadSearchResult(resp)
}
