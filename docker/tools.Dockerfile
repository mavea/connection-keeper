FROM golang:1.26

# Install Java runtime for PlantUML jar and helper tools used by scripts.
RUN apt-get update \
    && apt-get install -y --no-install-recommends default-jre-headless ca-certificates curl git graphviz fontconfig libfreetype6 libharfbuzz0b \
    && rm -rf /var/lib/apt/lists/*

# Install golangci-lint binary.
RUN GOBIN=/usr/local/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Install PlantUML jar to a stable path used by Makefile/scripts.
RUN curl -fsSL -o /opt/plantuml.jar https://github.com/plantuml/plantuml/releases/latest/download/plantuml.jar

WORKDIR /workspace
