-- Palhelm Pal Grant Bridge protocol v1. Server-only; vanilla clients.
-- The bridge stays fail-closed until one exact game build passes validation.
local MOD = "PalhelmPalBridge"
local PROTOCOL = 1

local function log(message)
    print(string.format("[%s] %s\n", MOD, tostring(message)))
end

local source = debug.getinfo(1, "S").source:gsub("^@", ""):gsub("\\", "/")
local separator = package.config and package.config:sub(1, 1) or "/"
local function joinPath(base, child)
    if base:sub(-1) == "/" or base:sub(-1) == "\\" then return base .. child end
    return base .. separator .. child
end
local scriptDir = source:match("^(.+)/[^/]+$")
local modDir = scriptDir and scriptDir:match("^(.+)/Scripts$") or nil
if not modDir then
    log("ERROR: cannot resolve mod directory")
    return
end

local configPath = joinPath(modDir, "config.lua")
local okConfig, config = pcall(dofile, configPath)
if not okConfig or type(config) ~= "table" then
    log("ERROR: config.lua is missing or invalid")
    return
end

-- Runtime policy is deliberately reloadable. UE4SS loads this script only at game
-- startup, but operators must be able to promote a successfully validated exact
-- build without restarting Palworld a second time. Immutable paths/catalogue
-- identity remain pinned to the startup configuration.
local immutableConfig = {
    SpoolDir = config.SpoolDir,
    GameVersion = config.GameVersion,
    CatalogVersion = config.CatalogVersion,
    CatalogItems = config.CatalogItems,
    MemberVariableLayoutPath = config.MemberVariableLayoutPath,
}

local function reloadRuntimeConfig()
    local ok, candidate = pcall(dofile, configPath)
    if not ok or type(candidate) ~= "table" then return false end
    for key, value in pairs(immutableConfig) do
        if candidate[key] ~= value then return false end
    end
    for _, key in ipairs({
        "GenerateUHTHeaders", "ValidationMode", "ValidationPreflightRequestID",
        "ValidationRequestID", "ValidationCharacterID", "ValidationLevel",
        "ValidationBoxOnly", "ProductionValidated", "AdvancedEnabled",
    }) do
        config[key] = candidate[key]
    end
    return true
end

local function safeToken(value)
    return type(value) == "string" and value:match("^[A-Za-z0-9_]+$") ~= nil and #value <= 160
end

local function safeVersion(value)
    return type(value) == "string" and value:match("^[A-Za-z0-9_.-]+$") ~= nil and #value <= 80
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

local function validObject(object)
    if not object then return false end
    local ok, valid = pcall(function() return object:IsValid() end)
    return ok and valid == true
end

local function probeFunction(path)
    if type(StaticFindObject) ~= "function" then return false end
    local ok, object = pcall(StaticFindObject, path)
    return ok and validObject(object)
end

local function fileExists(path)
    local file = io.open(path, "rb")
    if not file then return false end
    file:close()
    return true
end

local spool = config.SpoolDir or ""
local inbox = joinPath(spool, "inbox.json")
local processing = joinPath(spool, "processing.json")
local capability = joinPath(spool, "capability.json")
local defaultLayoutPath = joinPath(joinPath(joinPath(modDir, ".."), ".."), "MemberVariableLayout.ini")

local checks = {
    executeInGameThread = type(ExecuteInGameThread) == "function",
    findAllOf = type(FindAllOf) == "function",
    memberVariableLayout = fileExists(config.MemberVariableLayoutPath or defaultLayoutPath),
    debugCaptureNewMonster = probeFunction("/Script/Pal.PalPlayerState:Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer"),
    palStorage = probeFunction("/Script/Pal.PalPlayerState:GetPalStorage"),
    palParty = probeFunction("/Script/Pal.PalPlayerState:GetPalPlayerOtomoData"),
    palStorageCapacity = probeFunction("/Script/Pal.PalPlayerDataPalStorage:GetPageIndexExistEmptySlot"),
    palStorageSlots = probeFunction("/Script/Pal.PalIndividualCharacterContainer:GetSlots"),
    palContainerEmptySlot = probeFunction("/Script/Pal.PalIndividualCharacterContainer:FindEmptySlot"),
    palContainerManagerUtility = probeFunction("/Script/Pal.PalUtility:GetCharacterContainerManager"),
    palContainerManagerGet = probeFunction("/Script/Pal.PalCharacterContainerManager:GetContainer"),
}

local configured = type(spool) == "string" and spool ~= ""
    and safeVersion(config.GameVersion or "")
    and safeVersion(config.CatalogVersion or "")
    and tonumber(config.CatalogItems) ~= nil and tonumber(config.CatalogItems) > 0
    and checks.executeInGameThread and checks.findAllOf
    and checks.memberVariableLayout
    and checks.debugCaptureNewMonster and checks.palStorage and checks.palParty
    and checks.palStorageCapacity and checks.palStorageSlots
    and checks.palContainerEmptySlot and checks.palContainerManagerUtility
    and checks.palContainerManagerGet

local function utcNow()
    return os.date("!%Y-%m-%dT%H:%M:%SZ")
end

local function writeCapability()
    reloadRuntimeConfig()
    local state = "invalid_configuration"
    if configured then
        state = config.ProductionValidated == true and "ready" or "validation_required"
    end
    local body = string.format(
        '{"protocolVersion":%d,"state":"%s","gameVersion":"%s","catalogVersion":"%s","catalogItems":%d,"at":"%s"}\n',
        PROTOCOL, state, jsonEscape(config.GameVersion), jsonEscape(config.CatalogVersion),
        tonumber(config.CatalogItems) or 0, utcNow())
    writeAtomic(capability, body)
end

local function parseBoolean(body, name)
    return body:match('"' .. name .. '"%s*:%s*(true)') == "true"
end

local function parseOptionalNumber(body, name)
    local value = body:match('"' .. name .. '"%s*:%s*(%d+)')
    return value and tonumber(value) or nil
end

local function parseTokenArray(body, name, maximum)
    local encoded = body:match('"' .. name .. '"%s*:%s*%[([^%]]*)%]')
    if encoded == nil then return nil end
    if not encoded:match("%S") then return {} end
    local values = {}
    for value in encoded:gmatch('"([A-Za-z0-9_]+)"') do
        if not safeToken(value) or #values >= maximum then return nil end
        for _, existing in ipairs(values) do
            if string.lower(existing) == string.lower(value) then return nil end
        end
        values[#values + 1] = value
    end
    local remainder = encoded:gsub('"[A-Za-z0-9_]+"', ""):gsub("[%s,]", "")
    if remainder ~= "" or #values == 0 then return nil end
    return values
end

local function parseRequest(body)
    if not body then return nil, "request_unreadable" end
    local request = {
        protocolVersion = tonumber(body:match('"protocolVersion"%s*:%s*(%d+)')),
        requestId = body:match('"requestId"%s*:%s*"([A-Za-z0-9_]+)"'),
        playerUid = body:match('"playerUid"%s*:%s*"([A-Za-z0-9_]+)"'),
        characterId = body:match('"characterId"%s*:%s*"([A-Za-z0-9_]+)"'),
        mode = body:match('"mode"%s*:%s*"([A-Za-z0-9_]+)"'),
        levelMin = tonumber(body:match('"levelMin"%s*:%s*(%d+)')),
        levelMax = tonumber(body:match('"levelMax"%s*:%s*(%d+)')),
        gender = body:match('"gender"%s*:%s*"([A-Za-z0-9_]+)"'),
        alpha = parseBoolean(body, "alpha"),
        lucky = parseBoolean(body, "lucky"),
        preflightOnly = parseBoolean(body, "preflightOnly"),
        ivMode = body:match('"ivMode"%s*:%s*"([A-Za-z0-9_]+)"'),
        talentHp = parseOptionalNumber(body, "talentHp"),
        talentMelee = parseOptionalNumber(body, "talentMelee"),
        talentShot = parseOptionalNumber(body, "talentShot"),
        talentDefense = parseOptionalNumber(body, "talentDefense"),
        condensationStars = tonumber(body:match('"condensationStars"%s*:%s*(%d+)')),
        passiveSkillIds = parseTokenArray(body, "passiveSkillIds", 4),
    }
    if request.playerUid then request.playerUid = string.lower(request.playerUid) end
    if request.protocolVersion ~= PROTOCOL or not safeToken(request.requestId)
        or not request.playerUid or #request.playerUid ~= 32 or not request.playerUid:match("^%x+$")
        or not safeToken(request.characterId)
        or (request.mode ~= "natural" and request.mode ~= "custom")
        or not request.levelMin or not request.levelMax or request.levelMax < request.levelMin
        or request.levelMin < 1 or request.levelMax > 80
        or (request.gender ~= "random" and request.gender ~= "male" and request.gender ~= "female")
        or (request.alpha and request.lucky)
        or (request.ivMode ~= "natural" and request.ivMode ~= "minimum" and request.ivMode ~= "exact")
        or request.condensationStars == nil or request.condensationStars < 0 or request.condensationStars > 4
        or request.passiveSkillIds == nil then
        return request, "unsupported_or_invalid_request"
    end
    if request.ivMode == "natural" then
        if request.talentHp ~= nil or request.talentMelee ~= nil or request.talentShot ~= nil
            or request.talentDefense ~= nil then return request, "unsupported_or_invalid_request" end
    else
        if request.talentHp == nil or request.talentMelee == nil or request.talentShot == nil
            or request.talentDefense == nil then return request, "unsupported_or_invalid_request" end
        for _, value in ipairs({ request.talentHp, request.talentMelee, request.talentShot, request.talentDefense }) do
            if value < 0 or value > 100 then return request, "unsupported_or_invalid_request" end
        end
    end
    if request.mode == "natural" and (request.gender ~= "random" or request.alpha or request.lucky
        or request.ivMode ~= "natural" or #request.passiveSkillIds > 0 or request.condensationStars ~= 0) then
        return request, "unsupported_or_invalid_request"
    end
    if request.mode == "custom" and config.AdvancedEnabled ~= true then
        return request, "unsupported_or_invalid_request"
    end
    return request, nil
end

local function resultPath(requestId)
    return joinPath(joinPath(spool, "results"), requestId .. ".json")
end

local function writeFailure(request, code, message)
    local body = string.format(
        '{"protocolVersion":%d,"requestId":"%s","status":"failed","errorCode":"%s","errorMessage":"%s","completedAt":"%s"}\n',
        PROTOCOL, jsonEscape(request.requestId), jsonEscape(code), jsonEscape(message), utcNow())
    if not writeAtomic(resultPath(request.requestId), body) then
        log("ERROR: could not write failure result for " .. tostring(request.requestId))
        return false
    end
    os.remove(processing)
    return true
end

local function writePreflightSuccess(request)
    local body = string.format(
        '{"protocolVersion":%d,"requestId":"%s","status":"succeeded","preflightOnly":true,"playerUid":"%s","completedAt":"%s"}\n',
        PROTOCOL, jsonEscape(request.requestId), jsonEscape(request.playerUid), utcNow())
    if not writeAtomic(resultPath(request.requestId), body) then
        log("ERROR: could not write preflight result for " .. tostring(request.requestId))
        return false
    end
    os.remove(processing)
    return true
end

local function hex32(value)
    local formatted = string.format("%016x", tonumber(value) or 0)
    return formatted:sub(-8)
end

local function guidHex(value)
    local ok, result = pcall(function()
        return string.lower(hex32(value.A) .. hex32(value.B) .. hex32(value.C) .. hex32(value.D))
    end)
    return ok and result or ""
end

local function fNameString(value)
    local ok, result = pcall(function() return value:ToString() end)
    return ok and tostring(result) or tostring(value or "")
end

local function findController(uid)
    local controllers = FindAllOf("PalPlayerController")
    if not controllers then return nil end
    local match = nil
    for _, controller in ipairs(controllers) do
        if validObject(controller) then
            local ok, controllerUid = pcall(function() return guidHex(controller:GetPlayerUId()) end)
            if ok and controllerUid == uid then
                if match then return nil end
                match = controller
            end
        end
    end
    return match
end

local function addContainerHandles(handles, container)
    if not validObject(container) then error("character_container_unavailable") end
    local slots = container:GetSlots()
    if not slots then error("character_container_slots_unavailable") end
    for _, slot in ipairs(slots) do
        if validObject(slot) then
            local handle = slot:GetHandle()
            if validObject(handle) then
                local id = handle:GetIndividualID()
                local instanceId = guidHex(id.InstanceId)
                if instanceId ~= "" then handles[instanceId] = handle end
            end
        end
    end
end

local function ownedHandles(controller, requireEmptySlot, boxOnly)
    local state = controller:GetPalPlayerState()
    if not validObject(state) then error("player_state_unavailable") end
    local storage = state:GetPalStorage()
    if not validObject(storage) then error("pal_storage_unavailable") end
    local boxContainer = storage.TargetContainer
    if not validObject(boxContainer) then error("pal_storage_container_unavailable") end

    local handles = {}
    local partyContainer = nil
    local partyEmpty = false
    if not boxOnly then
        local partyData = state:GetPalPlayerOtomoData()
        if not validObject(partyData) then error("pal_party_data_unavailable") end
        local rawContainerId = partyData.OtomoCharacterContainerId
        local rawGuid = rawContainerId.ID
        local containerId = { ID = {
            A = tonumber(rawGuid.A), B = tonumber(rawGuid.B),
            C = tonumber(rawGuid.C), D = tonumber(rawGuid.D),
        } }
        local pawn = controller:GetDefaultPlayerCharacter()
        if not validObject(pawn) then error("player_character_unavailable") end
        local utility = StaticFindObject("/Script/Pal.Default__PalUtility")
        if not validObject(utility) then error("pal_utility_unavailable") end
        local manager = utility:GetCharacterContainerManager(pawn)
        if not validObject(manager) then error("character_container_manager_unavailable") end
        partyContainer = manager:GetContainer(containerId)
        if not validObject(partyContainer) then error("pal_party_container_unavailable") end
        partyEmpty = validObject(partyContainer:FindEmptySlot())
        addContainerHandles(handles, partyContainer)
    end

    if requireEmptySlot then
        local emptyPage = tonumber(storage:GetPageIndexExistEmptySlot(0))
        local boxEmpty = emptyPage ~= nil and emptyPage >= 0
        if not partyEmpty and not boxEmpty then error("owned_pal_containers_full") end
    end

    addContainerHandles(handles, boxContainer)
    return handles
end

local function passiveIDs(parameter)
    local out = {}
    local ok, values = pcall(function() return parameter:GetPassiveSkillList() end)
    if ok and values then
        for _, value in ipairs(values) do out[#out + 1] = fNameString(value) end
    end
    return out
end

local function parameterResult(request, instanceId, parameter)
    local characterId = fNameString(parameter:GetCharacterID())
    local normalizedCharacterId = characterId:gsub("^[Bb][Oo][Ss][Ss]_", "")
    local isAlpha = normalizedCharacterId ~= characterId
    if string.lower(normalizedCharacterId) ~= string.lower(request.characterId) then
        error("created_character_mismatch")
    end
    local genderCode = tonumber(parameter:GetGenderType())
    if genderCode == nil then genderCode = tonumber(tostring(parameter:GetGenderType()):match("(%d+)$")) end
    local gender = genderCode == 1 and "male" or genderCode == 2 and "female" or ""
    if gender == "" then error("created_gender_unknown") end
    local save = parameter.SaveParameter
    local talents = {
        hp = tonumber(save.Talent_HP), melee = tonumber(save.Talent_Melee),
        shot = tonumber(save.Talent_Shot), defense = tonumber(save.Talent_Defense),
    }
    for _, value in pairs(talents) do
        if value == nil or value < 0 or value > 100 then error("created_talent_unknown") end
    end
    local level = tonumber(parameter:GetLevel())
    local rank = tonumber(parameter:GetRank())
    if not level or not rank then error("created_level_or_rank_unknown") end
    if level < request.levelMin or level > request.levelMax then error("created_level_mismatch") end
    local passives = passiveIDs(parameter)
    if #passives > 4 then error("created_passives_invalid") end
    if request.mode == "custom" then
        if request.gender ~= "random" and gender ~= request.gender then error("created_gender_mismatch") end
        if isAlpha ~= request.alpha then error("created_alpha_mismatch") end
        if (parameter:IsRarePal() == true) ~= request.lucky then error("created_lucky_mismatch") end
        if rank ~= request.condensationStars + 1 then error("created_rank_mismatch") end
        if request.ivMode == "exact" and (talents.hp ~= request.talentHp
            or talents.melee ~= request.talentMelee or talents.shot ~= request.talentShot
            or talents.defense ~= request.talentDefense) then error("created_exact_talents_mismatch") end
        if request.ivMode == "minimum" and (talents.hp < request.talentHp
            or talents.melee < request.talentMelee or talents.shot < request.talentShot
            or talents.defense < request.talentDefense) then error("created_minimum_talents_mismatch") end
        if #passives ~= #request.passiveSkillIds then error("created_passives_mismatch") end
        local expected = {}
        for _, passive in ipairs(request.passiveSkillIds) do expected[string.lower(passive)] = true end
        for _, passive in ipairs(passives) do
            if not expected[string.lower(passive)] then error("created_passives_mismatch") end
        end
    end
    local encodedPassives = {}
    for _, passive in ipairs(passives) do encodedPassives[#encodedPassives + 1] = '"' .. jsonEscape(passive) .. '"' end
    return string.format(
        '{"protocolVersion":%d,"requestId":"%s","status":"succeeded","instanceId":"%s","level":%d,"gender":"%s","alpha":%s,"lucky":%s,"talentHp":%d,"talentMelee":%d,"talentShot":%d,"talentDefense":%d,"passiveSkillIds":[%s],"condensationStars":%d,"completedAt":"%s"}\n',
        PROTOCOL, jsonEscape(request.requestId), jsonEscape(instanceId), level, gender,
        tostring(isAlpha), tostring(parameter:IsRarePal() == true), talents.hp, talents.melee, talents.shot,
        talents.defense, table.concat(encodedPassives, ","), math.max(0, rank - 1), utcNow())
end

local function applyCustomOverrides(request, parameter)
    if request.mode ~= "custom" then return end
    local save = parameter.SaveParameter
    if request.gender == "male" then save.Gender = 1 end
    if request.gender == "female" then save.Gender = 2 end
    save.IsRarePal = request.lucky == true
    save.Rank = request.condensationStars + 1
    if request.ivMode == "exact" then
        save.Talent_HP = request.talentHp
        save.Talent_Melee = request.talentMelee
        save.Talent_Shot = request.talentShot
        save.Talent_Defense = request.talentDefense
    elseif request.ivMode == "minimum" then
        save.Talent_HP = math.max(tonumber(save.Talent_HP) or 0, request.talentHp)
        save.Talent_Melee = math.max(tonumber(save.Talent_Melee) or 0, request.talentMelee)
        save.Talent_Shot = math.max(tonumber(save.Talent_Shot) or 0, request.talentShot)
        save.Talent_Defense = math.max(tonumber(save.Talent_Defense) or 0, request.talentDefense)
    end
end

local busy = false
local function verifyGrant(request, controller, before)
    local attempts = 0
    LoopAsync(500, function()
        attempts = attempts + 1
        ExecuteInGameThread(function()
            local ok, outcome = pcall(function()
                local boxOnly = config.ProductionValidated ~= true and config.ValidationBoxOnly == true
                local after = ownedHandles(controller, false, boxOnly)
                local candidates = {}
                for id, handle in pairs(after) do
                    if not before[id] then candidates[#candidates + 1] = { id = id, handle = handle } end
                end
                if #candidates == 0 then return nil end
                if #candidates ~= 1 then error("multiple_new_pals_detected") end
                local parameter = candidates[1].handle:TryGetIndividualParameter()
                if not validObject(parameter) then return nil end
                applyCustomOverrides(request, parameter)
                return parameterResult(request, candidates[1].id, parameter)
            end)
            if ok and outcome then
                if writeAtomic(resultPath(request.requestId), outcome) then os.remove(processing) end
                busy = false
            elseif not ok then
                writeFailure(request, "verification_failed", tostring(outcome))
                busy = false
            elseif attempts >= 30 then
                writeFailure(request, "outcome_unknown", "Palworld did not expose one verified new Palbox specimen; this request will not be retried.")
                busy = false
            end
        end)
        return not busy or attempts >= 30
    end)
end

local function validationRequestAllowed(request)
    if config.ValidationMode ~= true then return false end
    local naturalProbe = request.mode == "natural" and request.levelMin == request.levelMax
        and request.gender == "random" and request.alpha == false and request.lucky == false
        and request.ivMode == "natural" and #request.passiveSkillIds == 0
        and request.condensationStars == 0
    if not naturalProbe then return false end
    if request.preflightOnly then
        return request.requestId == tostring(config.ValidationPreflightRequestID or "")
        and request.characterId == tostring(config.ValidationCharacterID or "")
        and request.levelMin == tonumber(config.ValidationLevel)
    end
    local preflight = readFile(resultPath(tostring(config.ValidationPreflightRequestID or ""))) or ""
    return request.requestId == tostring(config.ValidationRequestID or "")
        and request.characterId == tostring(config.ValidationCharacterID or "")
        and request.levelMin == tonumber(config.ValidationLevel)
        and preflight:match('"status"%s*:%s*"succeeded"') ~= nil
        and preflight:match('"preflightOnly"%s*:%s*true') ~= nil
        and preflight:match('"playerUid"%s*:%s*"' .. request.playerUid .. '"') ~= nil
end

local function captureMonsterForPlayer(playerState, request)
    local captureOk, captureError = pcall(function()
        local level = request.levelMin
        if request.levelMax > request.levelMin then
            local seed = tonumber(request.requestId:sub(1, 8), 16) or 0
            level = request.levelMin + (seed % (request.levelMax - request.levelMin + 1))
        end
        local characterId = request.alpha and ("BOSS_" .. request.characterId) or request.characterId
        local passiveRows = {}
        for _, passive in ipairs(request.passiveSkillIds) do
            passiveRows[#passiveRows + 1] = { Key = FName(passive) }
        end
        local info = {
            PalName = { Key = FName(characterId) },
            Level = level,
            Rank = request.condensationStars + 1,
            TalentLevel = 0,
            WazaList = {},
            PassiveSkill = passiveRows,
            StatusRank = {},
            FriendshipRank = 0,
            bIsAwakening = false,
        }
        playerState:Debug_CaptureNewMonsterByDebugOtomoInfo_ToServer(info, request.mode == "natural")
    end)
    if not captureOk then
        return false, "outcome_unknown", "The Palworld capture call returned an error after it began; it will not be retried: " .. tostring(captureError)
    end
    return true, nil, nil
end

local function processInbox()
    reloadRuntimeConfig()
    if busy or not configured or not readFile(inbox) then return end
    if not os.rename(inbox, processing) then return end
    local request, parseError = parseRequest(readFile(processing))
    if parseError then
        writeFailure(request or { requestId = "invalid" }, parseError, "The bridge rejected an invalid or unsupported Pal request.")
        return
    end
    if readFile(resultPath(request.requestId)) then
        os.remove(processing)
        return
    end
    if config.ProductionValidated ~= true and not validationRequestAllowed(request) then
        writeFailure(request, "validation_required", "This exact game build has not passed isolated Pal grant validation.")
        return
    end
    busy = true
    local scheduled, scheduleError = pcall(function()
        ExecuteInGameThread(function()
            local ok, outcome, controller, callErrorCode, callErrorMessage, preflightOnly = pcall(function()
                local resolvedController = findController(request.playerUid)
                if not resolvedController then
                    return nil, nil, "player_not_found", "No unique online player matched the stable UID on the game thread."
                end
                local boxOnly = config.ProductionValidated ~= true and config.ValidationBoxOnly == true
                local before = ownedHandles(resolvedController, true, boxOnly)
                if request.preflightOnly then
                    return before, resolvedController, nil, nil, true
                end
                local playerState = resolvedController:GetPalPlayerState()
                if not validObject(playerState) then error("player_state_unavailable") end
                local callOk, errorCode, errorMessage = captureMonsterForPlayer(playerState, request)
                if not callOk then return nil, resolvedController, errorCode, errorMessage end
                return before, resolvedController, nil, nil
            end)
            if not ok then
                writeFailure(request, "grant_call_failed", tostring(outcome))
                busy = false
            elseif not outcome then
                writeFailure(request, callErrorCode or "outcome_unknown", callErrorMessage or "The Palworld grant outcome could not be established.")
                busy = false
            elseif preflightOnly then
                writePreflightSuccess(request)
                busy = false
            else
                verifyGrant(request, controller, outcome)
            end
        end)
    end)
    if not scheduled then
        writeFailure(request, "outcome_unknown", "The game-thread call could not be scheduled; this request was not retried: " .. tostring(scheduleError))
        busy = false
    end
end

local orphan = parseRequest(readFile(processing))
if orphan then
    writeFailure(orphan, "outcome_unknown", "The game stopped while this grant was processing; it was not retried.")
end

local function writeDumpStatus(state, detail)
    local body = string.format('{"state":"%s","detail":"%s","at":"%s"}\n', jsonEscape(state), jsonEscape(detail), utcNow())
    writeAtomic(joinPath(spool, "uht-dump-status.json"), body)
end

local function generateUHTHeadersOnce()
    if config.GenerateUHTHeaders ~= true then return end
    if type(GenerateUHTCompatibleHeaders) ~= "function" then
        writeDumpStatus("unavailable", "GenerateUHTCompatibleHeaders is unavailable")
        return
    end
    writeDumpStatus("running", "read-only UHT header generation started")
    ExecuteInGameThread(function()
        local ok, err = pcall(GenerateUHTCompatibleHeaders)
        if ok then
            writeDumpStatus("complete", "inspect UE4SS/UHTHeaderDump")
        else
            writeDumpStatus("failed", tostring(err))
        end
    end)
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
LoopAsync(15000, function()
    generateUHTHeadersOnce()
    return true
end)

local loadedState = configured and (config.ProductionValidated == true and "ready" or "validation_required") or "invalid_configuration"
log(string.format("loaded: state=%s, capture-native grant=%s, advanced=%s", loadedState,
    tostring(checks.debugCaptureNewMonster), tostring(config.AdvancedEnabled == true)))
