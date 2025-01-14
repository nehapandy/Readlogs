package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EventLog struct {
	ID          int    `json:"id" bson:"id"`
	Provider    string `json:"providerName" bson:"providerName"`
	TimeCreated string `json:"timeCreated" bson:"timeCreated"`
	Level       string `json:"levelDisplayName" bson:"levelDisplayName"`
	Message     string `json:"message" bson:"message"`
}

var client *mongo.Client

func main() {
	var err error
	client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
		return
	}
	defer client.Disconnect(context.TODO())

	// Run log extraction

	storeLogsToMongo()

	router := gin.Default()
	router.GET("/logs", getLogsFromMongo)
	router.Run(":8000")
}

func storeLogsToMongo() {
	// PowerShell command to get system logs in JSON format
	cmd := exec.Command("powershell", "Get-WinEvent -LogName Application   | Select-Object -First 40 | Select-Object Id, ProviderName, TimeCreated, LevelDisplayName, Message| ConvertTo-Json -Depth 3")

	// Run the command and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error executing PowerShell command: %v", err)
		return
	}

	var logs []EventLog
	err = json.Unmarshal(output, &logs)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
		return
	}

	collection := client.Database("systemLogs").Collection("logs") // Insert logs into MongoDB

	var documents []interface{}
	for _, logEntry := range logs {
		documents = append(documents, logEntry)
	}

	_, err = collection.InsertMany(context.TODO(), documents)
	if err != nil {
		log.Fatalf("Error inserting logs into MongoDB: %v", err)
	}

	fmt.Println("Logs successfully stored in MongoDB!")
}

func getLogsFromMongo(c *gin.Context) {
	collection := client.Database("systemLogs").Collection("logs")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}

	skip := (page - 1) * limit

	// Define Find options
	findOptions := options.Find()
	findOptions.SetLimit(int64(limit))
	findOptions.SetSkip(int64(skip))
	findOptions.SetSort(bson.D{{"timeCreated", -1}}) //  newest first

	cursor, err := collection.Find(context.TODO(), bson.M{}, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching logs"})
		return
	}
	defer cursor.Close(context.TODO())

	var logs []EventLog
	if err := cursor.All(context.TODO(), &logs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding logs"})
		return
	}

	// Return logs as JSON response
	c.JSON(http.StatusOK, gin.H{
		"page":  page,
		"limit": limit,
		"data":  logs,
	})
}

