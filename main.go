package main

import (
	"encoding/json"
	"errors"
	"fmt"
    "log"
    "crypto/sha1"
	// "context"
    // "regexp"
    // "strings"
	"net/http"
	"os"
	// "strconv"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/events"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/dynamodb"
    "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

var (
	ErrorBackend = errors.New("Something went wrong")
    tableName = os.Getenv("SSW_CDB_TABLE")
    waitPeriod = float64(120000)
)
var errorLogger = log.New(os.Stderr, "ERROR ", log.Llongfile)
var DDB *dynamodb.DynamoDB

type RequestCoords struct {
	Lat float64 `json:"latitude"`
    Lng float64 `json:"longitude"`
}

type FeatureGeometry struct {
    Type string `json:"type"`
    Coordinates []interface{} `json:"coordinates"`
}

type FeatureProperties struct {
    Reporter string `json:"reporter"`
    Reported float64 `json:"reported"`
    Details string `json:"details"`
}

type Feature struct {
    Id string `json:"id"`
    Type string `json:"type"`
    Properties FeatureProperties `json:"properties"`
    Geometry FeatureGeometry `json:"geometry"`
}

type TypeConfig struct {
    Icy string `json:"icy,omitempty"`
    Plowed string `json:"plowed,omitempty"`
    Tree string `json:"tree,omitempty"`
    Lines string `json:"lines,omitempty"`
}

type Config struct {
    StrokeColor TypeConfig `json:"strokeColor,omitempty"`
    StrokeOpacity TypeConfig `json:"strokeOpacity,omitempty"`
    StrokeWeight TypeConfig `json:"strokeWeight,omitempty"`
}

type ClientResponse struct {
 	Features []Feature `json:"features"`
    Config Config `json:"config,omitempty"`
    WaitPeriod float64 `json:"waitPeriod"`
}

func init() {
    fmt.Println("init()")
    region := os.Getenv("AWS_REGION")
    if session, err := session.NewSession(&aws.Config{ // Use aws sdk to connect to dynamoDB
        Region: &region,
    }); err != nil {
        fmt.Println(fmt.Sprintf("Failed to connect to AWS: %s", err.Error()))
    } else {
        DDB = dynamodb.New(session) // Create DynamoDB client
    }
    fmt.Println(fmt.Sprintf("%+v", DDB))
}

func fetchConditions(centerCoords RequestCoords) ([]Feature, error) {
    var results []Feature

    input := &dynamodb.ScanInput{
        TableName: aws.String(tableName),
    }
    result, serr := DDB.Scan(input)
    if serr != nil {
        return nil, serr
    }

    for _, i := range result.Items {
        feature := Feature{}
        if err := dynamodbattribute.UnmarshalMap(i, &feature); err != nil {
            return nil, err
        }
        results = append(results, feature)
    }
    return results, nil
}

// func fetchConfig() (Config, error) {
//     var results Config

//     input := &dynamodb.ScanInput{
//         TableName: aws.String(tableName),
//     }
//     result, serr := DDB.Scan(input)
//     if serr != nil {
//         return nil, serr
//     }

//     for _, i := range result.Items {
//         feature := Feature{}
//         if err := dynamodbattribute.UnmarshalMap(i, &feature); err != nil {
//             return nil, err
//         }
//         results = append(results, feature)
//     }
//     return results, nil
// }

func putCondition(feature Feature) (error) {
    fmt.Println("putCondition()")
    // fmt.Printf("%+v\n", feature)
    av, err := dynamodbattribute.MarshalMap(feature)

    if err != nil {
        fmt.Sprintf("failed to DynamoDB marshal Record, %v", err)
        return err
    }

    current := &dynamodb.AttributeValue{
        N: aws.String("1"),
    }
    timelessFeature := feature
    timelessFeature.Properties.Reported = 0
    jsonObj, _ := json.Marshal(timelessFeature)
    hash := sha1.Sum(jsonObj)
    id := &dynamodb.AttributeValue{
        S: aws.String(fmt.Sprintf("%x", hash)),
    }

    av["current"] = current
    av["id"] = id

    input := &dynamodb.PutItemInput{
        TableName: aws.String(tableName),
        Item: av,
    }
    
    // fmt.Printf("input: %+v\n", input)
    fmt.Println("DDB.PutItem")
    _, perr := DDB.PutItem(input)
    if perr != nil {
        fmt.Printf("Got error on PutItem: %+v\n", perr)
        return perr
    }
    return nil
}

func handleGet(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    fmt.Printf("Requested path: %s\n", request.Path)
    return getConditions(request)
}

func handlePost(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    fmt.Printf("Requested path: %s\n", request.Path)
    return writeCondition(request)
}

func handleOptions(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    fmt.Printf("Requested path: %s\n", request.Path)
    response := events.APIGatewayProxyResponse{Headers: make(map[string]string)}

    response.StatusCode = http.StatusOK

    response.Headers["Access-Control-Allow-Origin"] = "*"
    response.Headers["Access-Control-Allow-Headers"] = "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers"
    response.Body = ""
    return response, nil
}

func writeCondition(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    var feature Feature

    response := events.APIGatewayProxyResponse{Headers: make(map[string]string)}

    response.StatusCode = http.StatusOK
    response.Headers["Access-Control-Allow-Origin"] = "*"
    
    for key, value := range request.Headers {
        fmt.Printf("    %s: %s\n", key, value)
    }

    fmt.Printf("Requested path: %s\n", request.Path)
    fmt.Printf("Request to write: %s\n", request.Body)

    err := json.Unmarshal([]byte(request.Body), &feature)

    if err != nil {
        fmt.Printf("Couldn not json.Unmarshal body: %+v", err)
        return response, err
    }

    err = putCondition(feature)

    if err != nil {
        return response, err
    }

    response.Body = request.Body
    return response, nil
}

func getConditions(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    var response events.APIGatewayProxyResponse

    response.StatusCode = http.StatusOK
    response.Body = ""

    for key, value := range request.Headers {
        fmt.Printf("    %s: %s\n", key, value)
    }

    fmt.Printf("Requested path: %s\n", request.Path)

    var requestCoords RequestCoords
    requestCoords.Lat = 0
    requestCoords.Lng = 0

    features, err := fetchConditions(requestCoords)
    var config Config

    config.StrokeWeight.Plowed = "6"

    if err == nil {
        fmt.Printf("---\n%+v\n", features)
        if response.Body == "" {
            var clientResponse ClientResponse
            clientResponse.Features = features
            clientResponse.WaitPeriod = waitPeriod
            clientResponse.Config= config
            jsonBody, err := json.Marshal(clientResponse)
            if err != nil {
                return serverError(err)
            }
            response.Body = fmt.Sprintf("sswfeed_callback(%s)", string(jsonBody))
        }
    }

    return response, nil
}

// Add a helper for handling errors. This logs any error to os.Stderr
// and returns a 500 Internal Server Error response that the AWS API
// Gateway understands.
func serverError(err error) (events.APIGatewayProxyResponse, error) {
    errorLogger.Println(err.Error())

    return events.APIGatewayProxyResponse{
        StatusCode: http.StatusInternalServerError,
        Body:       http.StatusText(http.StatusInternalServerError),
    }, nil
}

// Similarly add a helper for send responses relating to client errors.
func clientError(status int) (events.APIGatewayProxyResponse, error) {
    return events.APIGatewayProxyResponse{
        StatusCode: status,
        Body:       http.StatusText(status),
    }, nil
}

func router(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    jsonEvent, _ := json.Marshal(req)
    fmt.Println(string(jsonEvent))
    switch req.HTTPMethod {
    case "GET":
        return handleGet(req)
    case "POST":
        return handlePost(req)
    case "OPTIONS":
        return handleOptions(req)
    default:
        return clientError(http.StatusMethodNotAllowed)
    }
}

func main() {
	lambda.Start(router)
}