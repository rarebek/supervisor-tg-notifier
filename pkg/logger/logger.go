package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var LogFolder = "./logs"
var MaxLogAge = 60

func Log(folder string, msg ...interface{}) {

	t := time.Now()
	yy := fmt.Sprintf("%04d", t.Year())
	mm := fmt.Sprintf("%02d", t.Month())
	dd := fmt.Sprintf("%02d", t.Day())

	filePath := LogFolder + "/" + folder + "/" + yy + "/" + mm + "/" + dd

	err := os.MkdirAll(filePath, os.ModePerm)
	if err != nil {
		log.Println("Failed to create file path: ", err)
	}

	fileName := filePath + "/" + "tgnotify" + ".log"

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Println("Failed to open log file=", file, ":", err)
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Println(err)
		}
	}(file)

	log.SetOutput(file)
	log.Println(msg)
	log.SetOutput(os.Stdout)

	go cleanUpLogs(folder)

}

func GinLogFile() io.Writer {

	t := time.Now()
	yy := fmt.Sprintf("%04d", t.Year())
	mm := fmt.Sprintf("%02d", t.Month())
	dd := fmt.Sprintf("%02d", t.Day())

	filePath := LogFolder + "/gin/" + yy + "/" + mm + "/" + dd

	err := os.MkdirAll(filePath, os.ModePerm)
	if err != nil {
		log.Println("Failed to create file path: ", err)
	}

	fileName := filePath + "/" + "gin" + ".log"

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Println("Failed to open log file=", file, ":", err)
	}

	return file
}

func cleanUpLogs(folder string) {
	cutoffDate := time.Now().AddDate(0, 0, -MaxLogAge)
	err := filepath.Walk(filepath.Join(LogFolder, folder), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().Before(cutoffDate) {
			if err := os.Remove(path); err != nil {
				log.Println("Failed to delete log file:", err)
			}
		}

		return nil
	})

	if err != nil {
		log.Println("Error during log cleanup:", err)
	}
}
