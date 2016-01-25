package main

import (
	"os"
	"log"
	"time"
	"io/ioutil"
	"path/filepath"
	"encoding/xml"
	"database/sql"
	_ "gopkg.in/cq.v1"
)

type Gav struct {
	Group 		string `xml:"groupId"`
	Artifact 	string `xml:"artifactId"`
	Version 	string `xml:"version"`
	Scope 		string `xml:"scope"`
	Type 		string `xml:"type"`
}

type Pom struct {
	Group 		string `xml:"groupId"`
	Artifact 	string `xml:"artifactId"`
	Version 	string `xml:"version"`
	Packaging 	string `xml:"packaging"`
	
	Parent Gav `xml:"parent"`
	Dependencies []Gav `xml:"dependencies>dependency"`
}

var (
	db *sql.DB
	stmt *sql.Stmt
)

func main() {
	//get base dir for mvn repo
	repoBase := os.Args[1:2][0]
	log.Printf("Using %s as Maven Repository base directory", repoBase)
	
	establishNeo4j()
	
	//recurse through folders
	processedCount := scanFolder(repoBase)
	log.Printf("finished processing %v artifacts", processedCount)
	
	closingNeo4j()
}

func establishNeo4j() {
	var err error
	db, err = sql.Open("neo4j-cypher", "http://neo4j:7474")
    if err != nil {
        log.Printf("error establishing connection: %v", err)
		return
    }
	
	err = db.Ping()
	for retryCount := 0; err != nil && retryCount < 5; {
		log.Printf("connection could not be validated: %v ... retry in 2s", err)
		time.Sleep(2*time.Second)
		err = db.Ping()
		retryCount++
    }
	
	if err != nil {
        log.Fatal("error establishing connection: ", err)
    } else {
		log.Printf("successfully validated connection")
	}
}

func closingNeo4j() {
    if stmt != nil {
		stmt.Close()
	}
	
	if db != nil {
		db.Close()
	}
}


func scanFolder(basedir string) (count int) {
	log.Printf("Scanning directory %s", basedir)
	entries, err := ioutil.ReadDir(basedir)
	
	if err != nil {
		log.Printf("error reading directory %s: %v", basedir, err)
		return
	}
	
	log.Printf("found %v entries", len(entries))
	for i := 0; i<len(entries); i++ {
		log.Printf("processing item %v/%v: %s", i+1, len(entries), entries[i].Name())
		if entries[i].IsDir() {
			count += scanFolder(basedir + "/" + entries[i].Name())
		} else{
			count += processFile(basedir + "/" + entries[i].Name())
		}
	}

	return
}

func processFile(curFile string) int {	
	ext := filepath.Ext(curFile)

	if ext == ".pom" {
		pom := Pom{}
		
		content, err := ioutil.ReadFile(curFile)
		if err != nil {
			log.Printf("error reading file %s: %v", curFile, err)
			return 0
		}
		
		xml.Unmarshal(content, &pom)
		processArtifact(pom)
		return 1
	} else{
		log.Printf("skipping file %s due to wrong file extension (%s)", curFile, ext)
		return 0
	}
}

func processArtifact(pom Pom) {
	pom = initPom(pom)
	log.Printf("pocessing artifact (%s:%s:%s:%s) with %v dependencies", pom.Group, pom.Artifact, pom.Version, pom.Packaging, len(pom.Dependencies))

	result, err := db.Exec(`
		merge (p:pom {groupId:'` + pom.Parent.Group + `', artifactId:'` + pom.Parent.Artifact + `', version:'` + pom.Parent.Version + `'})
		merge (c:` + pom.Packaging + ` {groupId:'` + pom.Group + `', artifactId:'` + pom.Artifact + `', version:'` + pom.Version + `'})
		merge (p)-[:PARENT_OF]->(c)
	`)
	
    if err != nil {
        log.Printf("error executing statement: %v", err)
		return
    } else{
		updateCount, err := result.RowsAffected()
		if err != nil {
			log.Printf("error executing statement: %v", err)
			return
		}else{
			log.Printf("inserted %v nodes", updateCount)
		}
	}
    	
}

func initPom(pom Pom) Pom {
	if(pom.Packaging == ""){
		pom.Packaging = "jar"
	}
	
	if pom.Group == "" {
		pom.Group = pom.Parent.Group
	}

	if pom.Version == "" {
		pom.Version = pom.Parent.Version
	}

	return pom
}