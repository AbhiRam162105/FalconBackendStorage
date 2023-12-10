package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Query struct {
	Question string `json:"question,omitempty" bson:"question,omitempty"`
	Response string `json:"response,omitempty" bson:"response,omitempty"`
}

type Data struct {
	ID      primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Info    string             `json:"info,omitempty" bson:"info,omitempty"`
	Queries []Query            `json:"queries,omitempty" bson:"queries,omitempty"`
}

type Note struct {
	ID    primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Title string             `json:"title,omitempty" bson:"title,omitempty"`
	Text  string             `json:"text,omitempty" bson:"text,omitempty"`
	Data  Data               `json:"data,omitempty" bson:"data,omitempty"`
}

type Notebook struct {
	ID         primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Title      string             `json:"title,omitempty" bson:"title,omitempty"`
	Notes      []Note             `json:"notes,omitempty" bson:"notes,omitempty"`
	Created    time.Time          `json:"created,omitempty" bson:"created,omitempty"`
	LastAccess time.Time          `json:"lastAccess,omitempty" bson:"lastAccess,omitempty"`
	Data       Data               `json:"data,omitempty" bson:"data,omitempty"`
}

var client *mongo.Client

func getAllNotebooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var notebooks []Notebook
	collection := client.Database("testdb").Collection("notebooks")
	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var notebook Notebook
		if err := cursor.Decode(&notebook); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		notebooks = append(notebooks, notebook)
	}

	if err := cursor.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(notebooks)
}

//get all the notebook data

func getNotebookByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	id := params["id"]

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")

	err = collection.FindOneAndUpdate(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": bson.M{"lastAccess": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&notebook)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Notebook not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(notebook)
}

//get notebook by id ,returns all fields

func getNotebookByTitle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	title := params["title"]

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")

	err := collection.FindOne(context.Background(), bson.M{"title": title}).Decode(&notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	result := struct {
		ID         primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
		Title      string             `json:"title,omitempty" bson:"title,omitempty"`
		LastAccess time.Time          `json:"lastAccess,omitempty" bson:"lastAccess,omitempty"`
	}{ID: notebook.ID, Title: notebook.Title, LastAccess: notebook.LastAccess}

	json.NewEncoder(w).Encode(result)
}

//get notebook by Title ,returns ID,Title,Last Access

func patchUpdateNoteData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var data Data
	err = json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"notes.$.data": data, "lastAccess": time.Now()}}
	result, err := collection.UpdateOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.ModifiedCount == 0 {
		http.Error(w, "Note not found or no changes applied", http.StatusNotFound)
		return
	}

	// Retrieve the updated note data
	var updatedData Data
	err = collection.FindOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}).Decode(&updatedData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updatedData)
}

// changes note data
func postNotebook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var notebookReq struct {
		Title string `json:"title,omitempty"`
		Notes []Note `json:"notes,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&notebookReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	notebook := Notebook{
		Title:      notebookReq.Title,
		Notes:      notebookReq.Notes,
		Created:    time.Now(),
		LastAccess: time.Now(),
		Data:       Data{}, // You can initialize Data as needed
	}

	collection := client.Database("testdb").Collection("notebooks")
	_, err = collection.InsertOne(context.Background(), notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(notebook)
}

// creates a new notebook
func getLastAccessDate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]

	objectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")

	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := struct {
		LastAccess time.Time `json:"lastAccess,omitempty" bson:"lastAccess,omitempty"`
	}{LastAccess: notebook.LastAccess}

	json.NewEncoder(w).Encode(response)
}

func updateLastAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	id := params["id"]

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"lastAccess": time.Now()}}
	result, err := collection.UpdateOne(context.Background(), bson.M{"_id": objectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.ModifiedCount == 0 {
		http.Error(w, "Notebook not found or no changes applied", http.StatusNotFound)
		return
	}

	// Retrieve the updated last access time
	var updatedNotebook Notebook
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedNotebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		LastAccess time.Time `json:"lastAccess,omitempty" bson:"lastAccess,omitempty"`
	}{LastAccess: updatedNotebook.LastAccess}

	json.NewEncoder(w).Encode(response)
}

//updates the last access time of a notebook

func patchUpdateNotebookData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	id := params["id"]

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	var dataPatch struct {
		Title      *string    `json:"title,omitempty"`
		Notes      []Note     `json:"notes,omitempty"`
		Created    *time.Time `json:"created,omitempty"`
		LastAccess *time.Time `json:"lastAccess,omitempty"`
		Data       struct {
			Info    string  `json:"info,omitempty"`
			Queries []Query `json:"queries,omitempty"`
			Notes   []Note  `json:"notes,omitempty"`
		} `json:"data,omitempty"`
	}

	err = json.NewDecoder(r.Body).Decode(&dataPatch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updateFields := bson.M{}
	if dataPatch.Title != nil {
		updateFields["title"] = *dataPatch.Title
	}
	if len(dataPatch.Notes) > 0 {
		updateFields["notes"] = dataPatch.Notes
	}
	if dataPatch.Created != nil {
		updateFields["created"] = *dataPatch.Created
	}
	if dataPatch.LastAccess != nil {
		updateFields["lastAccess"] = *dataPatch.LastAccess
	}
	if dataPatch.Data.Info != "" || len(dataPatch.Data.Queries) > 0 || len(dataPatch.Data.Notes) > 0 {
		updateFields["data"] = dataPatch.Data
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": updateFields}
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": objectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the updated data
	var updatedNotebook Notebook
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedNotebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updatedNotebook)
}

//updates the data of a notebook

func patchUpdateNoteTitle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var noteTitle struct {
		Title string `json:"title,omitempty"`
	}

	err = json.NewDecoder(r.Body).Decode(&noteTitle)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{
		"$set": bson.M{
			"notes.$.title":      noteTitle.Title,
			"notes.$.lastAccess": time.Now(),
		},
	}
	result, err := collection.UpdateOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.ModifiedCount == 0 {
		http.Error(w, "Note not found or no changes applied", http.StatusNotFound)
		return
	}

	// Retrieve the updated note data
	var updatedNote Note
	err = collection.FindOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}).Decode(&updatedNote)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updatedNote)
}

//updates the note title and return the changed value

func removeNotebookByID(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.Error(w, "Invalid ID format", http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	_, err = collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func postNote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]

	objectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	var note Note
	err = json.NewDecoder(r.Body).Decode(&note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$push": bson.M{"notes": note}, "$set": bson.M{"lastAccess": time.Now()}}
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": objectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(note)
}

func removeNoteByID(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$pull": bson.M{"notes": bson.M{"_id": noteObjectID}}, "$set": bson.M{"lastAccess": time.Now()}}
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": notebookObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getDataByNotebookID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]

	objectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var allData []Data
	for _, note := range notebook.Notes {
		allData = append(allData, note.Data)
	}

	json.NewEncoder(w).Encode(allData)
}

func getDataByNoteAndID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")
	err = collection.FindOne(context.Background(), bson.M{"_id": notebookObjectID}).Decode(&notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	for _, note := range notebook.Notes {
		if note.ID == noteObjectID {
			json.NewEncoder(w).Encode(note.Data)
			return
		}
	}

	http.Error(w, "Note not found", http.StatusNotFound)
}

func postData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var data Data
	err = json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"notes.$.data": data, "lastAccess": time.Now()}}
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(data)
}

func removeAllNotebooks(w http.ResponseWriter, r *http.Request) {
	collection := client.Database("testdb").Collection("notebooks")
	_, err := collection.DeleteMany(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// getAllNotesByNotebookID gets all notes of a notebook by ID
func getAllNotesByNotebookID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]

	objectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&notebook)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(notebook.Notes)
}
func removeAllData(w http.ResponseWriter, r *http.Request) {
	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"notes.$[].data": Data{}}}
	_, err := collection.UpdateMany(context.Background(), bson.M{}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func removeDataByNotebookID(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	notebookID := params["notebookID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"notes.$[].data": Data{}}}
	_, err = collection.UpdateMany(context.Background(), bson.M{"_id": notebookObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func removeDataByNoteAndID(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{"$set": bson.M{"notes.$.data": Data{}}}
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": notebookObjectID, "notes._id": noteObjectID}, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getNoteByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var noteData struct {
		ID    primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
		Title string             `json:"title,omitempty" bson:"title,omitempty"`
		Text  string             `json:"text,omitempty" bson:"text,omitempty"`
	}

	collection := client.Database("testdb").Collection("notebooks")

	err = collection.FindOne(
		context.Background(),
		bson.M{"_id": notebookObjectID, "notes._id": noteObjectID},
		options.FindOne().SetProjection(bson.M{"notes.$.title": 1, "notes.$.text": 1}),
	).Decode(&noteData)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(noteData)
}

// Fetch Note by ID to get Id ,title ,text
func patchUpdateNoteText(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var newText struct {
		Text string `json:"text,omitempty"`
	}

	err = json.NewDecoder(r.Body).Decode(&newText)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	collection := client.Database("testdb").Collection("notebooks")
	update := bson.M{
		"$set": bson.M{
			"notes.$.text":       newText.Text,
			"notes.$.lastAccess": time.Now(),
		},
	}

	result, err := collection.UpdateOne(
		context.Background(),
		bson.M{"_id": notebookObjectID, "notes._id": noteObjectID},
		update,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.ModifiedCount == 0 {
		http.Error(w, "Note not found or no changes applied", http.StatusNotFound)
		return
	}

	// Retrieve the updated note data
	var updatedNote Note
	err = collection.FindOne(
		context.Background(),
		bson.M{"_id": notebookObjectID, "notes._id": noteObjectID},
	).Decode(&updatedNote)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updatedNote)
}

//Change Note Text and return the changed text

func getAllDataByNoteID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	notebookID := params["notebookID"]
	noteID := params["noteID"]

	notebookObjectID, err := primitive.ObjectIDFromHex(notebookID)
	if err != nil {
		http.Error(w, "Invalid Notebook ID format", http.StatusBadRequest)
		return
	}

	noteObjectID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		http.Error(w, "Invalid Note ID format", http.StatusBadRequest)
		return
	}

	var notebook Notebook
	collection := client.Database("testdb").Collection("notebooks")

	err = collection.FindOne(
		context.Background(),
		bson.M{"_id": notebookObjectID, "notes._id": noteObjectID},
	).Decode(&notebook)

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(notebook.Notes[0].Data)
}

//Fetch All Data of a Note by ID

func main() {
	var err error
	client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb+srv://abhiyanampally:aBHIRAMY@cluster0.erhdu89.mongodb.net/"))
	if err != nil {
		panic(err)
	}
	defer client.Disconnect(context.Background())

	router := mux.NewRouter()

	router.HandleFunc("/notebooks", getAllNotebooks).Methods("GET")
	router.HandleFunc("/notebooks/{id}", getNotebookByID).Methods("GET")
	router.HandleFunc("/notebooks/{title}", getNotebookByTitle).Methods("GET")

	router.HandleFunc("/notebooks", postNotebook).Methods("POST")
	router.HandleFunc("/notebooks/{id}", removeNotebookByID).Methods("DELETE")
	router.HandleFunc("/notebook/{notebookID}/notes", postNote).Methods("POST")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}", removeNoteByID).Methods("DELETE")
	router.HandleFunc("/notebooks/{notebookID}/lastaccess", updateLastAccess).Methods("POST")
	router.HandleFunc("/notebooks/{notebookID}/data", getDataByNotebookID).Methods("GET")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/data", getDataByNoteAndID).Methods("GET")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/data", postData).Methods("POST")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/data", removeDataByNoteAndID).Methods("DELETE")
	router.HandleFunc("/notebooks/{notebookID}/data", removeDataByNotebookID).Methods("DELETE")
	router.HandleFunc("/notebooks/removeAll", removeAllNotebooks).Methods("DELETE")
	router.HandleFunc("/notebooks/removeAllData", removeAllData).Methods("DELETE")
	router.HandleFunc("/notebooks/{id}/data", patchUpdateNotebookData).Methods("PATCH")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/data", patchUpdateNoteData).Methods("PATCH")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/title", patchUpdateNoteTitle).Methods("PATCH")
	router.HandleFunc("/notebooks/{notebookID}/lastaccessdate", getLastAccessDate).Methods("GET")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}", getNoteByID).Methods("GET")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/text", patchUpdateNoteText).Methods("PATCH")
	router.HandleFunc("/notebooks/{notebookID}/notes/{noteID}/alldata", getAllDataByNoteID).Methods("GET")
	router.HandleFunc("/notebooks/{notebookID}/notes", getAllNotesByNotebookID).Methods("GET")
	// CORS setup
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Update with your allowed origins
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Wrap the router with the CORS handler
	handler := corsHandler.Handler(router)

	log.Fatal(http.ListenAndServe(":8000", handler))
}
