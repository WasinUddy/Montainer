@real-image @lifecycle
Feature: Serialize lifecycle operations around a real Bedrock process
  As a Montainer release maintainer
  I want concurrent management requests to preserve one healthy server
  So that process races are detected before an image is promoted

  Scenario: Concurrent stop and start requests preserve one server
    Given the candidate Montainer image is available
    When I start the candidate with the packaged Bedrock server
    Then the packaged Bedrock server eventually reports running
    And a RakNet client can discover the Bedrock server
    When I request 8 stops concurrently
    Then exactly one lifecycle request succeeds and the others conflict
    And the packaged Bedrock server eventually reports stopped
    When I request 8 starts concurrently
    Then exactly one lifecycle request succeeds and the others conflict
    And the process generation increases by 1
    And a RakNet client can eventually discover the Bedrock server
