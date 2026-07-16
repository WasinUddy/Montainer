@real-image @smoke
Feature: Boot the packaged Mojang Bedrock server
  As a Montainer release maintainer
  I want the candidate image to boot the scraped Mojang binary
  So that an incompatible server archive is never promoted

  Scenario: The candidate exposes a playable Bedrock endpoint
    Given the candidate Montainer image is available
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And the local logs contain the expected Bedrock version
    And a RakNet client can discover the Bedrock server
    When I send the real server command "real-image-smoke-token"
    Then the local logs eventually contain "real-image-smoke-token"
