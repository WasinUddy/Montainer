@real-image @backup
Feature: Back up a real Bedrock world to S3-compatible storage
  As a Montainer release maintainer
  I want the packaged server to survive a consistent backup cycle
  So that Mojang lifecycle changes cannot silently break world protection

  Scenario: Concurrent backup requests create one valid MinIO archive
    Given the candidate Montainer image is available
    And S3-compatible MinIO storage is available
    When I start the candidate with the packaged Bedrock server
    Then a RakNet client can eventually discover the Bedrock server
    When I request 4 backups concurrently
    Then exactly one backup succeeds and the others conflict
    And the uploaded backup is a valid Montainer archive
    And the process generation increases by 1
    And a RakNet client can eventually discover the Bedrock server
