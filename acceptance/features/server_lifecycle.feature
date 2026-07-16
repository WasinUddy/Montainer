@acceptance @lifecycle
Feature: Manage the Bedrock server lifecycle
  As a server administrator
  I want Montainer to own the Bedrock process lifecycle
  So that at most one game server is running and its real state is visible

  Background:
    Given Montainer uses a controllable fake Bedrock server
    And OpenTelemetry log export is disabled
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"

  Scenario: Stop and start the server
    Given fake Bedrock has eventually started 1 time
    When I request the server to stop
    Then the HTTP response status is 200
    And the server eventually reports "stopped"
    And fake Bedrock eventually receives the command "stop"
    When I request the server to start
    Then the HTTP response status is 200
    And the server eventually reports "running"
    And fake Bedrock has eventually started 2 times
    And fake Bedrock never overlaps another instance

  Scenario: Restart replaces the running process gracefully
    Given fake Bedrock has eventually started 1 time
    When I request the server to restart
    Then the HTTP response status is 200
    And the server eventually reports "running"
    And fake Bedrock eventually receives the command "stop"
    And fake Bedrock has eventually started 2 times
    And fake Bedrock never overlaps another instance

  Scenario: An unexpected Bedrock exit updates the reported state
    When I send the server command "crash 23"
    Then the HTTP response status is 200
    And the server eventually reports "stopped"
