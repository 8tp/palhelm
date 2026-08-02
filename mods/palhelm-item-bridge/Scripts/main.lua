-- Palhelm Item Bridge protocol v1. Server-only; vanilla clients.
local MOD = "PalhelmItemBridge"
local PROTOCOL = 1

local function log(message)
    print(string.format("[%s] %s\n", MOD, tostring(message)))
end

local source = debug.getinfo(1, "S").source:gsub("^@", ""):gsub("\\", "/")
local separator = package.config and package.config:sub(1, 1) or "\\"
local function joinPath(base, child)
    if base:sub(-1) == "/" or base:sub(-1) == "\\" then
        return base .. child
    end
    return base .. separator .. child
end

local scriptDir = source:match("^(.+)/[^/]+$")
local modDir = scriptDir and scriptDir:match("^(.+)/Scripts$") or nil
if not modDir then
    log("ERROR: cannot resolve mod directory")
    return
end

local okConfig, config = pcall(dofile, joinPath(modDir, "config.lua"))
if not okConfig or type(config) ~= "table" then
    log("ERROR: config.lua is missing or invalid; copy config.example.lua first")
    return
end

local function safeToken(value)
    return type(value) == "string" and value:match("^[A-Za-z0-9_]+$") ~= nil and #value <= 160
end

local function safeVersion(value)
    return type(value) == "string" and value:match("^[A-Za-z0-9_.-]+$") ~= nil and #value <= 40
end

local function jsonEscape(value)
    return tostring(value or ""):gsub("\\", "\\\\"):gsub('"', '\\"'):gsub("\r", "\\r"):gsub("\n", "\\n")
end

local function readFile(path)
    local file = io.open(path, "rb")
    if not file then return nil end
    local body = file:read("*a")
    file:close()
    if not body or body == "" or #body > 65536 then return nil end
    return body
end

local function writeAtomic(path, body)
    local temporary = path .. ".tmp"
    local file = io.open(temporary, "wb")
    if not file then return false end
    file:write(body)
    file:close()
    os.remove(path)
    if not os.rename(temporary, path) then
        os.remove(temporary)
        return false
    end
    return true
end

local allowlist = {}
local allowlistCount = 0
local allowlistVersion = ""
local allowlistBody = readFile(config.AllowlistPath or "")
if allowlistBody then
    allowlistVersion = allowlistBody:match("# gameVersion=([^\r\n]+)") or ""
    local declaredCount = tonumber(allowlistBody:match("# catalogItems=(%d+)") or "")
    for line in allowlistBody:gmatch("[^\r\n]+") do
        local itemId, maximum = line:match("^([A-Za-z0-9_]+)%s+(%d+)$")
        if itemId and maximum then
            allowlist[itemId] = tonumber(maximum)
            allowlistCount = allowlistCount + 1
        end
    end
    if declaredCount ~= allowlistCount then
        log("ERROR: allowlist count does not match its header")
        allowlist = {}
        allowlistCount = 0
    end
end

local configurationChecks = {
    { "spool_dir", type(config.SpoolDir) == "string" and config.SpoolDir ~= "" },
    { "game_version", safeVersion(config.GameVersion or "") },
    { "allowlist_version", allowlistVersion == config.GameVersion },
    { "allowlist_items", allowlistCount > 0 },
    { "inventory_signature", config.InventorySignatureArgs == 4 or config.InventorySignatureArgs == 5 },
    { "execute_in_game_thread", type(ExecuteInGameThread) == "function" },
    { "find_all_of", type(FindAllOf) == "function" },
    -- UE4SS 3.x exposes constructors as functions on Windows while the
    -- experimental native-Linux build exposes FName as callable userdata.
    { "fname", type(FName) == "function" or type(FName) == "userdata" or type(FName) == "table" },
}
local configured = true
local failedChecks = {}
for _, check in ipairs(configurationChecks) do
    if not check[2] then
        configured = false
        failedChecks[#failedChecks + 1] = check[1]
    end
end
if not configured then
    log("ERROR: configuration checks failed: " .. table.concat(failedChecks, ", "))
end

local spool = config.SpoolDir or ""
local inbox = joinPath(spool, "inbox.json")
local processing = joinPath(spool, "processing.json")
local capability = joinPath(spool, "capability.json")

local function utcNow()
    return os.date("!%Y-%m-%dT%H:%M:%SZ")
end

local function writeCapability()
    local state = "invalid_configuration"
    if configured then
        state = config.ProductionValidated == true and "ready" or "validation_required"
    end
    local body = string.format(
        '{"protocolVersion":%d,"state":"%s","gameVersion":"%s","catalogVersion":"%s","catalogItems":%d,"at":"%s"}\n',
        PROTOCOL, state, jsonEscape(config.GameVersion), jsonEscape(allowlistVersion), allowlistCount, utcNow())
    writeAtomic(capability, body)
end

local function parseRequest(body)
    if not body then return nil, "request_unreadable" end
    local request = {
        protocolVersion = tonumber(body:match('"protocolVersion"%s*:%s*(%d+)')),
        requestId = body:match('"requestId"%s*:%s*"([A-Za-z0-9_]+)"'),
        playerUid = body:match('"playerUid"%s*:%s*"([A-Za-z0-9_]+)"'),
        itemId = body:match('"itemId"%s*:%s*"([A-Za-z0-9_]+)"'),
        quantity = tonumber(body:match('"quantity"%s*:%s*(%d+)')),
    }
    if request.protocolVersion ~= PROTOCOL or not safeToken(request.requestId)
        or not request.playerUid or #request.playerUid ~= 32 or not request.playerUid:match("^%x+$")
        or not safeToken(request.itemId) or not request.quantity then
        return request, "request_invalid"
    end
    local maximum = allowlist[request.itemId]
    if not maximum or request.quantity < 1 or request.quantity > maximum then
        return request, "item_not_allowlisted"
    end
    return request, nil
end

local function resultPath(requestId)
    return joinPath(joinPath(spool, "results"), requestId .. ".json")
end

local function writeResult(request, status, granted, code, message)
    local body = string.format(
        '{"protocolVersion":%d,"requestId":"%s","status":"%s","grantedQuantity":%d,"errorCode":"%s","errorMessage":"%s","completedAt":"%s"}\n',
        PROTOCOL, jsonEscape(request.requestId), status, granted or 0,
        jsonEscape(code), jsonEscape(message), utcNow())
    if not writeAtomic(resultPath(request.requestId), body) then
        log("ERROR: could not write result for " .. request.requestId .. "; processing file retained")
        return false
    end
    os.remove(processing)
    return true
end

local function hex32(value)
    local formatted = string.format("%016x", tonumber(value) or 0)
    return formatted:sub(-8)
end

local function playerUid(playerState)
    local ok, uid = pcall(function()
        local value = playerState.PlayerUId
        return string.lower(hex32(value.A) .. hex32(value.B) .. hex32(value.C) .. hex32(value.D))
    end)
    return ok and uid or ""
end

local function findPlayer(uid)
    local states = FindAllOf("PalPlayerState")
    if not states then return nil end
    local match = nil
    for _, state in ipairs(states) do
        local valid = false
        pcall(function() valid = state:IsValid() end)
        if valid and playerUid(state) == uid then
            if match then return nil end -- duplicate identity: fail closed
            match = state
        end
    end
    return match
end

local busy = false
local function processInbox()
    if busy or not configured then return end
    if not readFile(inbox) then return end
    if not os.rename(inbox, processing) then return end
    local request, parseError = parseRequest(readFile(processing))
    if parseError then
        local fallback = { requestId = request and request.requestId or "invalid" }
        writeResult(fallback, "failed", 0, parseError, "The bridge rejected an invalid request.")
        return
    end
    if readFile(resultPath(request.requestId)) then
        os.remove(processing)
        return
    end
    if config.ProductionValidated ~= true then
        writeResult(request, "failed", 0, "validation_required", "This exact game build has not passed isolated bridge validation.")
        return
    end
    local state = findPlayer(request.playerUid)
    if not state then
        writeResult(request, "failed", 0, "player_not_found", "No unique online player matched the stable UID.")
        return
    end

    busy = true
    local scheduled, scheduleError = pcall(function()
        ExecuteInGameThread(function()
            local ok, operationResult = pcall(function()
                local inventory = state:GetInventoryData()
                if not inventory or not inventory:IsValid() then error("inventory_unavailable") end
                if config.InventorySignatureArgs == 5 then
                    return inventory:AddItem_ServerInternal(FName(request.itemId), request.quantity, false, 0.0, true)
                end
                return inventory:AddItem_ServerInternal(FName(request.itemId), request.quantity, false, 0.0)
            end)
            if not ok then
                writeResult(request, "failed", 0, "inventory_call_failed", tostring(operationResult))
            else
                local resultCode = tonumber(operationResult)
                if resultCode == nil then
                    resultCode = tonumber(tostring(operationResult):match("(%d+)$"))
                end
                if resultCode == 0 then
                    writeResult(request, "succeeded", request.quantity, "", "")
                elseif resultCode == nil then
                    writeResult(request, "failed", 0, "outcome_unknown", "The inventory result could not be interpreted; the request will not be retried.")
                else
                    writeResult(request, "failed", 0, "inventory_rejected", "Palworld inventory operation result " .. tostring(resultCode))
                end
            end
            busy = false
        end)
    end)
    if not scheduled then
        writeResult(request, "failed", 0, "outcome_unknown", "The game-thread call could not be scheduled; the request was not retried: " .. tostring(scheduleError))
        busy = false
    end
end

-- A processing file surviving process death has an unknowable mutation outcome.
-- Never execute it again.
local orphan = parseRequest(readFile(processing))
if orphan then
    writeResult(orphan, "failed", 0, "outcome_unknown", "The game stopped while this grant was processing; it was not retried.")
end

writeCapability()
LoopAsync(1000, function()
    processInbox()
    return false
end)
LoopAsync(10000, function()
    writeCapability()
    return false
end)

local loadedState = "invalid_configuration"
if configured then
    loadedState = config.ProductionValidated == true and "ready" or "validation_required"
end
log(string.format("loaded: state=%s, allowlisted items=%d", loadedState, allowlistCount))
