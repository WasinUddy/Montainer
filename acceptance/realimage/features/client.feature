@real-image @client
Feature: Join the packaged server with a virtual Bedrock player
  As a Montainer release maintainer
  I want a protocol-aware client to spawn and receive movement
  So that deeper Mojang compatibility failures are caught on stable releases

  Scenario: An offline player joins and receives the requested teleport
    Given the candidate Montainer image is available
    When I start the candidate with the packaged Bedrock server
    Then a RakNet client can eventually discover the Bedrock server
    When the virtual Bedrock player joins
    Then the local logs eventually contain "Player connected"
    When I send the real server command "list"
    Then the local logs eventually contain "MontainerCI"
    When I send the real server command "tp MontainerCI 12000 100 -12000"
    Then the virtual Bedrock player receives the teleport
