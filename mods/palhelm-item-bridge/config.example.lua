return {
    -- Both processes must see the same physical directory. Native Linux example:
    SpoolDir = [[/palhelm-item-grants]],
    AllowlistPath = [[/palhelm-item-grants/item-allowlist.txt]],
    -- Windows/Wine example:
    -- SpoolDir = [[C:\PalhelmData\item-grants]],
    -- AllowlistPath = [[C:\PalhelmData\item-allowlist.txt]],

    -- Must exactly match the catalogue and running dedicated-server build.
    GameVersion = "1.0.1",

    -- Current public 1.0 SDK exposes five parameters. Never auto-fallback after
    -- a failed call: a fallback could duplicate an item whose first result was
    -- merely unreadable.
    InventorySignatureArgs = 5,

    -- Fail closed until an isolated copy of this exact game build passes the
    -- checklist in README.md.
    ProductionValidated = false,
}
