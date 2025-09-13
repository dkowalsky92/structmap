package main

import (
	"flag"
	"go/format"
	"log"
	"os"
	"path/filepath"

	"github.com/dkowalsky92/structmap/internal/generator"
	"gopkg.in/yaml.v3"
)

func main() {
	configFile := flag.String("config", "", "YAML config file")
	conversionsFile := flag.String("conversions", "", "YAML conversions file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("usage: structmap -config config.yaml")
	}

	if *conversionsFile == "" {
		log.Fatal("usage: structmap -conversions conversions.yaml")
	}

	var cfg generator.Config
	raw, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		log.Fatal(err)
	}

	var conversions generator.Conversions
	raw, err = os.ReadFile(*conversionsFile)
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &conversions); err != nil {
		log.Fatal(err)
	}

	generator := generator.NewGenerator(cfg, conversions)
	code, err := generator.Generate()
	if err != nil {
		log.Fatal(err)
	}

	formattedCode, err := format.Source([]byte(code))
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Debug {
		log.Printf("Generated code:\n%s", code)
	}

	outFilePath := cfg.OutFilePath
	if outFilePath == "" {
		outFilePath = "."
	}
	outFileName := cfg.OutFileName
	if outFileName == "" {
		outFileName = "structmap.gen.go"
	}
	outputPath := filepath.Join(outFilePath, outFileName)
	if err := os.MkdirAll(outFilePath, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, formattedCode, 0644); err != nil {
		log.Fatal(err)
	}
}
