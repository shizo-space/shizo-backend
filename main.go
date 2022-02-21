package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/olivere/elastic/v7"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"net/http"

	"github.com/go-playground/validator"
	"github.com/gorilla/mux"
	b58 "github.com/mr-tron/base58"
	"github.com/paulmach/orb/encoding/mvt"
	"github.com/rs/cors"
	"golang.org/x/crypto/ed25519"
)

func getClient(url string, sniff bool) *elastic.Client {
	client, err := elastic.NewClient(
		elastic.SetSniff(sniff),
		elastic.SetURL(url),
		elastic.SetHealthcheckInterval(5*time.Second), // quit trying after 5 seconds
	)
	if err != nil {
		panic(err)
	}
	return client
}

type Coordinate struct {
	Lat         float64 `json:"lat"`
	Long        float64 `json:"long"`
}

type ResultRow struct {
	Name        string  	`json:"name"`
	Description string  	`json:"description"`
    LeftTop		Coordinate  `json:"left_top"`
	RightBottom Coordinate  `json:"right_bottom"`
	MergeId     string  	`json:"merge_id"`
	Importance  float64 	`json:"importance"`
}

type SearchServer struct {
	client *elastic.Client
	index  string
}


type FeatureSigner struct {
	s *SearchServer
	privateKey []byte
}

type FeatureInterceptor struct {
	db       *gorm.DB
	s        *SearchServer
	mbTileDB *MBTileDB
	nearInteractor *NearInteractor
}

type IndexableElement struct {
	Name          []string         `json:"name"`
	Modified_name string           `json:"modified_name"`
	Merge_id      string           `json:"merge_id"`
	Tagged_name   string           `json:"tagged_name"`
	BoundingBox   [4]float64	   `json:"bounding_box,omitempty"`
	Importance    float64          `json:"importance"`
	View          uint64           `json:"view"`
	IsBuilding 	  bool			   `json:"is_building"`
}

type ExtendedFeatureDto struct {
	Feature
	View         uint64 		`json:"view"`
	OsmName		 string 		`json:"osm_name"`
    LeftTop		 Coordinate  	`json:"left_top"`
	RightBottom  Coordinate  	`json:"right_bottom"`
	IsBuilding 	 bool	 		`json:"is_building"`
}

type SignatureDto struct {
	Signature	string 		`json:"signature" validate:"required"`
}

type FeatureList struct {
	MergeIds	[]string	`json:"merge_ids" validate:"omitempty"`
}

func search(client *elastic.Client, index string, query string, lat float64, long float64) []IndexableElement {
	result := make([]IndexableElement, 0)
	es_query := fmt.Sprintf(`
		{
			"bool": {
				"should": [
					{"match": {
						"name": {
							"query": "%s",
							"boost": 100
						}
					}},
					{"match": {
						"name.edge_ngram": {
							"query": "%s",
							"boost": 10
						}
					}},
					{"match": {
						"modified_name": {
							"query": "%s",
							"boost": 1000
						}
					}},
					{"match": {
						"modified_name.edge_ngram": {
							"query": "%s",
							"boost": 10
						}
					}}
				]
			}
		}

	`, query, query, query, query)

	fs_query := elastic.NewRawStringQuery(fmt.Sprintf(`{
		"function_score": {
		"query": %s,
		"functions": [{
			"field_value_factor": {
				"field": "importance",
				"factor": 1
			  }}
		],
		"boost_mode": "multiply"
	  }}`, es_query))
	searchResult, err := client.Search().Index(index).Query(fs_query).From(0).Size(10).Do(context.Background())
	if err != nil {
		panic(err)
	}
	var ttyp IndexableElement
	for _, item := range searchResult.Each(reflect.TypeOf(ttyp)) {
		if t, ok := item.(IndexableElement); ok {
			result = append(result, t)
		}
	}
	return result
}

func (s *SearchServer) handleGet(w http.ResponseWriter, req *http.Request) {
	result := make([]ResultRow, 0)

	query := mux.Vars(req)["q"]
	lat, err := strconv.ParseFloat(mux.Vars(req)["lat"], 64)
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
	long, err := strconv.ParseFloat(mux.Vars(req)["long"], 64)
	if err != nil {
		http.Error(w, err.Error(), 400)
	}
	for _, item := range search(s.client, s.index, query, lat, long) {

		
		name := ""
		if item.Modified_name != ""  && item.Name[0] != ""{
			name = fmt.Sprintf("%s (%s)", item.Modified_name, item.Name[0])
		} else if item.Modified_name != "" {
			name = item.Modified_name
		} else if item.Name[0] != "" {
			name = item.Name[0]
		}


		result = append(result, ResultRow{
			Name: name,
			Description: name,
			LeftTop: Coordinate{Lat: item.BoundingBox[2], Long: item.BoundingBox[3]},
			RightBottom: Coordinate{Lat: item.BoundingBox[0], Long: item.BoundingBox[1]},
			MergeId:     item.Merge_id,
			Importance:  item.Importance,
		})
	}

	json.NewEncoder(w).Encode(result)
}

var validate *validator.Validate



func UserStructLevelValidation(sl validator.StructLevel) {
    embeddedLinkRegexPatterns := []string{"^(?:https?:\\/\\/)?(?:m\\.|www\\.)?(?:youtu\\.be\\/|youtube\\.com\\/(?:embed\\/|v\\/|watch\\?v=|watch\\?.+&v=))((\\w|-){11})(?:\\S+)?$"}
	validColors := map[string]bool{"#AAE0FA": true, "#57C8FF": true, "#189EFF": true,
								   "#0047FF": true, "#561BFF": true, "#AD5AFF": true,
								   "#FFCA08": true, "#F7941D": true, "#F25822": true,
								   "#D8DF20": true, "#71BF45": true, "#00A65E": true,
								   "#F5F5F5": true, "#BDBDBD": true, "#808080": true,
								   "#606060": true, "#303030": true, "#101010": true,
								}

	feature := sl.Current().Interface().(Feature)

	if (feature.Color != "") && (! validColors[feature.Color]) {
		sl.ReportError(feature.Color, "color", "Color", "not valid option", "")
	}

	if(feature.EmbeddedLink != "" ) {
        found := false

		for _, pattern := range embeddedLinkRegexPatterns {

			match, _ := regexp.MatchString(pattern, feature.EmbeddedLink)

			if match {
				found = true
				break
			}
		}
		if !found {
			sl.ReportError(feature.EmbeddedLink, "embedded_link", "EmbeddedLink", "not a twitter or youtube link", "")
		}
	}
	// plus can do more, even with different tag than "fnameorlname"
}

func (fi FeatureInterceptor) GetTile(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	z, _ := vars["z"]
	x, _ := vars["x"]
	y, _ := vars["y"]

	z_num, _ := strconv.Atoi(z)
	x_num, _ := strconv.Atoi(x)
	y_num, _ := strconv.Atoi(y)

	tile, _ := fi.mbTileDB.GetTileData(uint8(z_num), uint64(x_num), uint64(y_num))
	layers, _ := mvt.UnmarshalGzipped(tile)

	mergeIds := make([]string, 0)

	for _, l := range layers {
		for _, f := range l.Features {
			if mergeId, mIdOK := f.Properties["merge_id"]; mIdOK {
				mergeIds = append(mergeIds, mergeId.(string))
			}
		}
	}

	features := make([]Feature, 0)

	fi.db.Where("merge_id In ?", mergeIds).Select("merge_id", "name", "color").Find(&features)

	featuresMap := make(map[string]string)
	mergeIdToColor :=  make(map[string]string)

	for _, feat := range features {
		featuresMap[feat.MergeId] = feat.Name
		mergeIdToColor[feat.MergeId] = feat.Color
	}

	for _, l := range layers {
		for _, f := range l.Features {
			mergeId, mIdOK := f.Properties["merge_id"]
			if !mIdOK {
				continue
			}

			if newName, ok := featuresMap[mergeId.(string)]; ok && newName != ""{
					f.Properties["name"] = newName
			}


			if color, ok := mergeIdToColor[mergeId.(string)]; ok && color != "" {
				_, isBuilding := f.Properties["building"]

				if isBuilding {
					f.Properties["color"] = color
				}
			}

		}
	}

	resultTile, _ := mvt.MarshalGzipped(layers)
	rw.Header().Set("Content-Type", "x-protobuf")
	rw.Header().Set("Content-Encoding", "gzip")
	rw.Write(resultTile)
}

func (s *SearchServer) getElasticElement(mergeId string) (*IndexableElement, bool) {

	var result IndexableElement
	searchResult, err := s.client.Get().Index(s.index).Id(mergeId).Do(context.Background())
	if err != nil {
		println(err)
		return nil, false
	}

	if searchResult.Found != true {
		return nil, false
	}
	err = json.Unmarshal(searchResult.Source, &result)

	if err != nil {
		panic(err)
	}

	return &result, true
}

func (s *SearchServer) updateView(element *IndexableElement) error {

	element.View = element.View + 1
	_, err := s.client.Update().Index(s.index).Id(element.Merge_id).Doc(element).Do(context.Background())

	return err
}

func (s *SearchServer) updateModifiedName(newName string, element *IndexableElement) error {

	element.Modified_name = newName
	_, err := s.client.Update().Index(s.index).Id(element.Merge_id).Doc(element).Do(context.Background())

	return err
}


func publicKeyFromString(s string) (publicKey []byte, err error) {
	b, err := b58.Decode(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, err
	}
	return b, nil
}

func (fi *FeatureInterceptor) ValicateSignatureIsByTheOwner(signatureDto SignatureDto, mergeId string) bool {
	owner := fi.nearInteractor.getOwnerByTokenId(mergeId)
	ownerPublicKeys := fi.nearInteractor.getAcountPublicKeys(owner)

	for _, pk := range ownerPublicKeys {
		publicKey, err := publicKeyFromString(pk)
		if err != nil {
			continue
		}
		matched := ed25519.Verify(publicKey, []byte(mergeId), []byte(signatureDto.Signature))
		if matched {
			return true
		}
	}
	return false
}

func (fi *FeatureInterceptor) UpdateFeature(rw http.ResponseWriter, req *http.Request) {
	var err error
	vars := mux.Vars(req)
	mergeId, _ := vars["mergeId"]


	elasticElement, found := fi.s.getElasticElement(mergeId)
	if !found {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	buf, err := ioutil.ReadAll(req.Body)
	rdr1 := ioutil.NopCloser(bytes.NewBuffer(buf))

	var signatureDto SignatureDto
	json.NewDecoder(rdr1).Decode(&signatureDto)

	validate = validator.New()

	err = validate.Struct(signatureDto)

	if err != nil {
		http.Error(rw, "Request Error "+err.Error(), http.StatusBadRequest)
		return
	}
	isSignatureValid := fi.ValicateSignatureIsByTheOwner(signatureDto, mergeId)

	if !isSignatureValid {
			rw.WriteHeader(http.StatusBadRequest)
			return
	}

	var feature Feature
	feature.MergeId = mergeId
	rdr2 := ioutil.NopCloser(bytes.NewBuffer(buf))
	json.NewDecoder(rdr2).Decode(&feature)

	if err != nil {
		http.Error(rw, "Request Error "+err.Error(), http.StatusBadRequest)
		return
	}

	validate = validator.New()
	validate.RegisterStructValidation(UserStructLevelValidation, Feature{})

	feature.Name = strings.TrimSpace(feature.Name)
	err = validate.Struct(feature)

	if err != nil {
		http.Error(rw, "Request Error "+err.Error(), http.StatusBadRequest)
		return
	}

	fi.s.updateModifiedName(feature.Name, elasticElement)

	fi.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "merge_id"}},                                                               // key colume
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "embedded_link", "color", "link_to_vr"}), // column needed to be updated
	}).Create(&feature)

	rw.WriteHeader(http.StatusNoContent)
	rw.Write([]byte{})
}


func (fi *FeatureInterceptor) GetExtendedFeature(elasticElement *IndexableElement) *ExtendedFeatureDto {
	var feature ExtendedFeatureDto
	res := fi.db.Model(&Feature{}).First(&feature, "merge_id = ?", elasticElement.Merge_id)

	if res.Error != nil {
		feature = ExtendedFeatureDto{Feature: Feature{MergeId: elasticElement.Merge_id}}
	}

	feature.View = elasticElement.View
	feature.OsmName = elasticElement.Name[0]
	feature.LeftTop = Coordinate{Lat: elasticElement.BoundingBox[0], Long: elasticElement.BoundingBox[1]}
	feature.RightBottom = Coordinate{Lat: elasticElement.BoundingBox[2], Long: elasticElement.BoundingBox[3]}
	feature.IsBuilding = elasticElement.IsBuilding
	
	return &feature
}

func (fi *FeatureInterceptor) GetFeature(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mergeId, _ := vars["mergeId"]

	elasticElement, found := fi.s.getElasticElement(mergeId)

	if !found {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	fi.s.updateView(elasticElement)
    
	feature := fi.GetExtendedFeature(elasticElement)

	rw.WriteHeader(http.StatusOK)
	body, _ := json.Marshal(feature)
	rw.Write(body)
}

func (fi *FeatureInterceptor) ListFeatures(rw http.ResponseWriter, req *http.Request) {

	var dto FeatureList
	err := json.NewDecoder(req.Body).Decode(&dto)
	if err != nil {
		http.Error(rw, "Request Error "+err.Error(), http.StatusBadRequest)
		return
	}

	validate = validator.New()

	err = validate.Struct(dto)

	if err != nil {
		http.Error(rw, "Request Error "+err.Error(), http.StatusBadRequest)
		return
	}

	features := make([]ExtendedFeatureDto, 0)

	for _, mergeId := range dto.MergeIds {
		elasticElement, found := fi.s.getElasticElement(mergeId)

		if found {
			feature := fi.GetExtendedFeature(elasticElement)
			features = append(features, *feature)
		}
	}

	rw.WriteHeader(http.StatusOK)
	body, _ := json.Marshal(&features)
	rw.Write(body)
}

func (featureSigner *FeatureSigner) GetFeatureSignature(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mergeId, _ := vars["mergeId"]


	_, found := featureSigner.s.getElasticElement(mergeId)
	if !found {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	data := []byte(mergeId)
	signature := ed25519.Sign(featureSigner.privateKey[:], data)
	signatureB58 := b58.Encode(signature[:])

	rw.WriteHeader(http.StatusOK)
	body, _ := json.Marshal(&SignatureDto{Signature: signatureB58})
	rw.Write(body)
}

func main() {
	var (
		url         = flag.String("url", "http://localhost:9200", "Elasticsearch URL")
		index       = flag.String("index", "dashaq", "Elasticsearch index name")
		sniff       = flag.Bool("sniff", true, "Enable or disable sniffing")
		mbtilesPath = flag.String("mbtiles", "/home/shinzo/Workspace/Personal/sibel-back/out/toronto-iterative-motorway-v4.mbtiles", "mbtiles path")
		dbPassword  = flag.String("dbPassword", "shizo", "db password")
		nearPrivateKey  = flag.String("nearPrivateKey", "3xnCUnp51K8YhVMF492cpEHJNufwdiRjpUrRnurDYaJ7FHKx2XUcAXatNNcAkzquxdp5AJVkayiZAw5A9TR4wqes", "near private key")
		NearRPCNode	= flag.String("nearRPCNode", "https://rpc.testnet.near.org", "near rpc node adress")
		NearMasterAccountId = flag.String("nearMasterAccountId", "shizotest.testnet", "near master account Id")
	)

	flag.Parse()

	dsn := fmt.Sprintf("host=localhost user=shizo password=%s dbname=shizo port=5432 sslmode=disable TimeZone=Etc/UTC", *dbPassword)
	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	db.AutoMigrate(&Feature{})

	client := getClient(*url, *sniff)

	mbTileDB, _ := NewDB(*mbtilesPath)

	searchServer := SearchServer{client: client, index: *index}

	nearInteractor := NearInteractor{RPCNode: *NearRPCNode, MasterAccountId: *NearMasterAccountId}

	featureInterceptor := FeatureInterceptor{db: db, mbTileDB: mbTileDB, s: &searchServer, nearInteractor: &nearInteractor}
	
	privateKey, _ := b58.Decode(*nearPrivateKey)

	featureSigner := FeatureSigner{s: &searchServer, privateKey: privateKey}


	r := mux.NewRouter()

	r.HandleFunc("/tiles/{z}/{x}/{y}", featureInterceptor.GetTile).Methods("GET")
	r.HandleFunc("/features/{mergeId}/", featureInterceptor.UpdateFeature).Methods("PUT")
	r.HandleFunc("/features/{mergeId}/", featureInterceptor.GetFeature).Methods("GET")
	r.HandleFunc("/features/{mergeId}/signature/", featureSigner.GetFeatureSignature).Methods("GET")
	r.HandleFunc("/features/list/", featureInterceptor.ListFeatures).Methods("POST")
	r.HandleFunc("/search/", searchServer.handleGet).
		Queries("q", "{q}").
		Queries("lat", "{lat}").
		Queries("long", "{long}").
		Methods("GET")

	handler := cors.AllowAll().Handler(r)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}
	srv.ListenAndServe()
}
