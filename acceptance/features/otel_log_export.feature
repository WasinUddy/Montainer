@acceptance @otel
Feature: Optionally export Bedrock logs with OpenTelemetry
  As a server operator
  I want to fan out logs to an OTLP collector when one is configured
  While keeping the local management experience independent of that collector

  Background:
    Given Montainer uses a controllable fake Bedrock server

  Scenario: Export a stdout log while preserving the local copy
    Given an OTLP HTTP collector is available
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"
    When I send the server command "emit exported-stdout-token"
    Then the local log API eventually contains "exported-stdout-token"
    And the OTLP collector eventually contains "exported-stdout-token"
    And the exported log "exported-stdout-token" has resource attribute "service.name" equal to "montainer"
    And the exported log "exported-stdout-token" has resource attribute "service.instance.id" equal to "acceptance-instance"
    And the exported log "exported-stdout-token" has attribute "log.iostream" equal to "stdout"

  Scenario: Export stderr as a distinguishable log stream
    Given an OTLP HTTP collector is available
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"
    When I send the server command "emiterr exported-stderr-token"
    Then the local log API eventually contains "exported-stderr-token"
    And the OTLP collector eventually contains "exported-stderr-token"
    And the exported log "exported-stderr-token" has attribute "log.iostream" equal to "stderr"

  Scenario: An unavailable collector cannot take down server management
    Given the configured OTLP endpoint is unavailable
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"
    When I send the server command "emit collector-outage-token"
    Then the HTTP response status is 200
    And the local log API eventually contains "collector-outage-token"
    And the management API remains healthy
    When I request the server to stop
    Then the HTTP response status is 200
    And the server eventually reports "stopped"

  Scenario: Application shutdown flushes pending exported logs
    Given an OTLP HTTP collector is available
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"
    When I send the server command "emit shutdown-flush-token"
    And I terminate Montainer
    Then Montainer exits cleanly
    And fake Bedrock eventually receives the command "stop"
    And the OTLP collector eventually contains "shutdown-flush-token"
