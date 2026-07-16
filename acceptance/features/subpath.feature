@acceptance @subpath
Feature: Manage a server through a configured URL subpath
  As an operator hosting multiple Montainer instances
  I want every management and streaming route scoped to its configured prefix
  So that instances can share a host without exposing duplicate root routes

  Scenario: Use the complete management contract below a prefix
    Given Montainer uses a controllable fake Bedrock server
    And OpenTelemetry log export is disabled
    And Montainer is served under the subpath "/servers/friends"
    When Montainer starts
    Then the management API eventually becomes healthy
    And the instance name endpoint returns "acceptance-instance"
    And the server eventually reports "running"
    When I request the server to toggle
    Then the HTTP response status is 200
    And the server eventually reports "stopped"
    And fake Bedrock eventually receives the command "stop"
    When I request the server to toggle
    Then the HTTP response status is 200
    And the server eventually reports "running"
    Given I am connected to the web log stream
    When I send the server command "emit prefixed-websocket-token"
    Then the HTTP response status is 200
    And the web log stream eventually contains "prefixed-websocket-token"
    And the unprefixed management route "/status" returns 404
