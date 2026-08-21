@real-image @upgrade
Feature: Run an existing Bedrock world under any ownership
  As a Montainer release maintainer
  I want the runtime identity to follow whoever owns the world data
  So that an ownership assumption cannot make an existing server unusable

  Scenario: A root-owned world remains playable and backup-safe after upgrade
    Given the candidate Montainer image is available
    And S3-compatible MinIO storage is available
    And a genuine root-owned legacy world exists on named volumes
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the persistence data still belongs to UID and GID 0
    And Montainer PID 1 and Bedrock run as UID 0 and GID 0
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
    And the candidate reports no filesystem permission errors
    When I restore the uploaded backup into fresh named volumes
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the candidate reports no filesystem permission errors
    When I stop the candidate container
    Then the candidate container exits cleanly

  Scenario: A world owned by an arbitrary host UID runs without being rewritten
    Given the candidate Montainer image is available
    And a genuine legacy world owned by UID and GID 1234 exists on named volumes
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the persistence data still belongs to UID and GID 1234
    And Montainer PID 1 and Bedrock run as UID 1234 and GID 1234
    And the candidate container eventually becomes healthy
    And the candidate reports no filesystem permission errors
    When I stop the candidate container
    Then the candidate container exits cleanly

  Scenario: PUID and PGID override the identity taken from the world data
    Given the candidate Montainer image is available
    And a genuine root-owned legacy world exists on named volumes
    And the candidate is configured with PUID 1500 and PGID 1600
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the legacy scoreboard state is preserved
    And the persistence data belongs to UID 1500 and GID 1600
    And Montainer PID 1 and Bedrock run as UID 1500 and GID 1600
    And the candidate reports no filesystem permission errors

  Scenario: A root-owned custom instance is writable before config restoration
    Given the candidate Montainer image is available
    And a root-owned custom pre-v3 Bedrock instance exists
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And Montainer PID 1 and Bedrock run as UID 0 and GID 0
    And the candidate reports no filesystem permission errors

  Scenario: Explicit non-root execution keeps the image health probe working
    Given the candidate Montainer image is available
    And the candidate is configured for explicit non-root execution
    When I start the candidate with the packaged Bedrock server
    Then the management API eventually becomes healthy
    And the packaged Bedrock server eventually reports running
    And a RakNet client can eventually discover the Bedrock server
    And the candidate container eventually becomes healthy
    And Montainer PID 1 and Bedrock run as UID 10001 and GID 10001
    And the candidate reports no filesystem permission errors
