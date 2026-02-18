build:
	go build -v -o test-importer ./importer
	go build -v -o test-exporter ./exporter

create:
	plakar pkg create manifest.yaml v1.1.0-beta.4
