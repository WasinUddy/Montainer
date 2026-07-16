@acceptance @logging
Feature: View Bedrock logs without external infrastructure
  As a server administrator
  I want logs to remain available through Montainer itself
  So that observability services are optional

  Background:
    Given Montainer uses a controllable fake Bedrock server
    And OpenTelemetry log export is disabled
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"

  Scenario: Standalone logging works with no collector configured
    When I send the server command "emit standalone-log-token"
    Then the HTTP response status is 200
    And the local log API eventually contains "standalone-log-token"
    And the management API remains healthy

  Scenario: Both Bedrock output streams remain visible locally
    When I send the server command "emit stdout-log-token"
    And I send the server command "emiterr stderr-log-token"
    Then the local log API eventually contains "stdout-log-token"
    And the local log API eventually contains "stderr-log-token"

  Scenario: The web console stream receives new Bedrock logs
    Given I am connected to the web log stream
    When I send the server command "emit websocket-log-token"
    Then the web log stream eventually contains "websocket-log-token"
