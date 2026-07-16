package realimage_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type bedrockAdvertisement struct {
	MOTD     string
	Protocol int
	Version  string
	Raw      string
}

func (s *scenarioState) rakNetEventuallyResponds() error {
	return eventually("RakNet discovery response from packaged Bedrock", realImageTimeout, func() error {
		advertisement, err := s.pingBedrockInNetwork()
		if err != nil {
			return err
		}
		if advertisement.Protocol <= 0 || strings.TrimSpace(advertisement.Version) == "" {
			return fmt.Errorf("incomplete Bedrock advertisement: %q", advertisement.Raw)
		}
		s.lastAdvertisement = advertisement
		return nil
	})
}

func (s *scenarioState) pingBedrockInNetwork() (bedrockAdvertisement, error) {
	if s.candidate == "" {
		return bedrockAdvertisement{}, fmt.Errorf("candidate container is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := s.docker(
		ctx,
		"run", "--rm",
		"--network", s.networkName,
		"--entrypoint", "/tmp/raknet-probe",
		"--volume", s.suite.probeBinary+":/tmp/raknet-probe:ro",
		s.suite.image,
		s.candidate+":19132",
	)
	if err != nil {
		return bedrockAdvertisement{}, err
	}
	var raw string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "MCPE;") {
			raw = strings.TrimSpace(line)
		}
	}
	if raw == "" {
		return bedrockAdvertisement{}, fmt.Errorf("RakNet probe returned no Bedrock advertisement: %q", output)
	}
	return parseBedrockAdvertisement(raw)
}

func parseBedrockAdvertisement(raw string) (bedrockAdvertisement, error) {
	fields := strings.Split(raw, ";")
	if len(fields) < 4 || fields[0] != "MCPE" {
		return bedrockAdvertisement{}, fmt.Errorf("unexpected Bedrock advertisement %q", raw)
	}
	protocol, err := strconv.Atoi(fields[2])
	if err != nil {
		return bedrockAdvertisement{}, fmt.Errorf("parse advertised protocol %q: %w", fields[2], err)
	}
	return bedrockAdvertisement{
		MOTD:     fields[1],
		Protocol: protocol,
		Version:  fields[3],
		Raw:      raw,
	}, nil
}
