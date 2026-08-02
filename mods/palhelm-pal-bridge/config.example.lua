return {
    SpoolDir = "/palhelm-data/pal-grants",
    GameVersion = "v1.0.1.100619",
    CatalogVersion = "palworld_1.0_pinned",
    CatalogItems = 249,
    -- Maintenance-only, read-only reflection dump. Set true for one startup,
    -- inspect UHTHeaderDump, then set false again.
    GenerateUHTHeaders = false,
    -- Isolated one-request validation gate. This does not make capability ready.
    ValidationMode = false,
    ValidationPreflightRequestID = "",
    ValidationRequestID = "",
    ValidationCharacterID = "SheepBall",
    ValidationLevel = 1,
    ValidationBoxOnly = false,
    -- Load custom generation support during the same game startup. The
    -- independent ProductionValidated gate still blocks ordinary requests.
    AdvancedEnabled = true,
    -- Optional override; normally auto-detected beside the ue4ss Mods directory.
    -- MemberVariableLayoutPath = "/absolute/path/to/ue4ss/MemberVariableLayout.ini",
    ProductionValidated = false,
}
