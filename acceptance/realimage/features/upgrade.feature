@real-image @upgrade
Feature: Upgrade a legacy root-owned Bedrock world
  As a Montainer release maintainer
  I want pre-v3 named volumes to migrate without losing their world
  So that an ownership change cannot make an existing server unusable

  Scenario: A root-owned world remains playable and backup-safe after upgrade
    Given the candidate Montainer image is available
    And S3-compatible MinIO storage is available
    And a genuine root-owned legacy world exists on named volumes
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the upgraded persistence data belongs to UID and GID 10001
    And Montainer PID 1 and Bedrock run as UID 10001
    When the virtual Bedrock player joins
    And I send the real server command "tp MontainerCI 12000 100 -12000"
    Then the virtual Bedrock player receives the teleport
    And the candidate reports no filesystem permission errors

    When I request 4 backups concurrently
    Then exactly one backup succeeds and the others conflict
    And the uploaded backup is a valid Montainer archive
    And the uploaded backup retains the legacy world database and canary
    And the process generation increases by 1
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And Montainer PID 1 and Bedrock run as UID 10001
    And the candidate reports no filesystem permission errors
    When I restore the uploaded backup into fresh named volumes
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the upgraded persistence data belongs to UID and GID 10001
    And Montainer PID 1 and Bedrock run as UID 10001
    And the candidate reports no filesystem permission errors
    When I stop the candidate container
    Then the candidate container exits cleanly

  Scenario: A root-owned custom instance is writable before config restoration
    Given the candidate Montainer image is available
    And a root-owned custom pre-v3 Bedrock instance exists
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the custom instance and persistence data belong to UID and GID 10001
    And Montainer PID 1 and Bedrock run as UID 10001
    And the candidate reports no filesystem permission errors

  Scenario: Explicit non-root execution keeps the image health probe working
    Given the candidate Montainer image is available
    And the candidate is configured for explicit non-root execution
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the candidate container eventually becomes healthy
    And Montainer PID 1 and Bedrock run as UID 10001
    And the candidate reports no filesystem permission errors

  Scenario: A same-device nested mount is never included in ownership migration
    Given the candidate Montainer image is available
    And an accessible root-owned nested persistence mount exists
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the candidate container eventually becomes healthy
    And the parent data migrates while the nested mount stays root-owned
    And the candidate reports no filesystem permission errors
    When I stop the candidate container
    Then the candidate container exits cleanly
