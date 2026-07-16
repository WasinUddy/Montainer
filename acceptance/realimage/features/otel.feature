@real-image @otel
Feature: Export real Bedrock logs through an OpenTelemetry Collector
  As a Montainer release maintainer
  I want the packaged server output to traverse the configured Collector
  While Collector failures remain isolated from normal operation

  @otel-export
  Scenario: Real server output reaches the pinned Collector
    Given the candidate Montainer image is available
    And a real OpenTelemetry Collector is available
    When I start the candidate with the packaged Bedrock server
    Then a RakNet client can eventually discover the Bedrock server
    When I send the real server command "real-image-otel-token"
    Then the local logs eventually contain "real-image-otel-token"
    And the Collector eventually contains "real-image-otel-token"
    And the Collector export contains the Montainer service identity

  @otel-outage
  Scenario: A Collector outage cannot break the real server
    Given the candidate Montainer image is available
    And the configured OpenTelemetry Collector is unavailable
    When I start the candidate with the packaged Bedrock server
    Then a RakNet client can eventually discover the Bedrock server
    When I send the real server command "real-image-otel-outage-token"
    Then the local logs eventually contain "real-image-otel-outage-token"
    And the management API remains healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server

  @otel-flush
  Scenario: Graceful container shutdown flushes pending real server logs
    Given the candidate Montainer image is available
    And a real OpenTelemetry Collector is available
    And log export batching is delayed until shutdown
    When I start the candidate with the packaged Bedrock server
    Then a RakNet client can eventually discover the Bedrock server
    When I send the real server command "real-image-shutdown-flush-token"
    Then the local logs eventually contain "real-image-shutdown-flush-token"
    When I stop the candidate container
    Then the candidate container exits cleanly
    And the Collector eventually contains "real-image-shutdown-flush-token"
