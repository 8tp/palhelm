-- One-shot in-memory adapter for the currently loaded script. Future starts use
-- the matching implementation already installed in main.lua.
local hotfixMarker = [[/palhelm-pal-grants/hotfix-owned-handles-once]]
local marker = io.open(hotfixMarker, "rb")
if marker then
    marker:close()
    os.remove(hotfixMarker)

    local function validObject(object)
        if not object then return false end
        local ok, valid = pcall(function() return object:IsValid() end)
        return ok and valid == true
    end

    local function hex32(value)
        local formatted = string.format("%016x", tonumber(value) or 0)
        return formatted:sub(-8)
    end

    local function instanceId(handle)
        local id = handle:GetIndividualID().InstanceId
        return string.lower(hex32(id.A) .. hex32(id.B) .. hex32(id.C) .. hex32(id.D))
    end

    local function addHandle(handles, handle)
        if not validObject(handle) then return false end
        local id = instanceId(handle)
        if #id ~= 32 then return false end
        handles[id] = handle
        return true
    end

    local function addContainer(handles, container)
        local slots = container:GetSlots()
        if not slots then error("character_container_slots_unavailable") end
        for _, slot in ipairs(slots) do
            if validObject(slot) then addHandle(handles, slot:GetHandle()) end
        end
    end

    local function ownedHandles(controller, requireEmptySlot)
        local state = controller:GetPalPlayerState()
        if not validObject(state) then error("player_state_unavailable") end
        local storage = state:GetPalStorage()
        if not validObject(storage) then error("pal_storage_unavailable") end
        local boxContainer = storage.TargetContainer
        if not validObject(boxContainer) then error("pal_storage_container_unavailable") end

        local handles = {}
        addContainer(handles, boxContainer)
        if requireEmptySlot then
            local emptyPage = tonumber(storage:GetPageIndexExistEmptySlot(0))
            local boxEmpty = emptyPage ~= nil and emptyPage >= 0
            if not boxEmpty then error("pal_storage_full") end
        end
        return handles
    end

    local patchedOwnedHandles = false
    local clearedBusy = false
    for stackLevel = 2, 12 do
        local info = debug.getinfo(stackLevel, "f")
        if info and type(info.func) == "function" then
            for index = 1, 128 do
                local name, value = debug.getupvalue(info.func, index)
                if not name then break end
                if name == "busy" then
                    debug.setupvalue(info.func, index, false)
                    clearedBusy = true
                elseif name == "ownedHandles" and type(value) == "function" then
                    debug.setupvalue(info.func, index, ownedHandles)
                    patchedOwnedHandles = true
                end
            end
        end
    end

    local diagnostic = io.open([[/palhelm-pal-grants/hotfix-owned-handles-result.txt]], "wb")
    if diagnostic then
        diagnostic:write("patchedOwnedHandles=", tostring(patchedOwnedHandles), " clearedBusy=", tostring(clearedBusy), "\n")
        diagnostic:close()
    end
end

return {
    SpoolDir = [[/palhelm-pal-grants]],
    GameVersion = "v1.0.1.100619",
    CatalogVersion = "palworld_1.0_pinned",
    CatalogItems = 249,
    GenerateUHTHeaders = false,
    ValidationMode = true,
    ValidationPreflightRequestID = "palhelm_validation_20260719_12_preflight",
    ValidationRequestID = "palhelm_validation_20260719_12",
    ValidationCharacterID = "SheepBall",
    ValidationLevel = 1,
    ValidationBoxOnly = true,
    AdvancedEnabled = true,
    ProductionValidated = false,
}
