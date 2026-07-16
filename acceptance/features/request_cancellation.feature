@acceptance @lifecycle @cancellation
Feature: Finish lifecycle operations after an HTTP client disconnects
  As a server administrator
  I want an accepted stop operation to have a server-owned lifetime
  So that a disconnected browser cannot force-kill or strand Bedrock

  Scenario: Cancel the HTTP request during a delayed graceful stop
    Given Montainer uses a controllable fake Bedrock server
    And OpenTelemetry log export is disabled
    And fake Bedrock takes "1s" to finish a graceful stop
    When Montainer starts
    Then the management API eventually becomes healthy
    And the server eventually reports "running"
    When I cancel a stop request after fake Bedrock receives it
    Then fake Bedrock eventually receives the command "stop"
    And the server eventually reports "stopped"
    And fake Bedrock has eventually exited gracefully 1 time
    And fake Bedrock received no operating system signal
    And fake Bedrock is eventually no longer active
