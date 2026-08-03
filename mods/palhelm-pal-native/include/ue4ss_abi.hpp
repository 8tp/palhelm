#pragma once

// Minimal ABI mirror for the exact Linux UE4SS fork installed on BES Pals.
//
// This is intentionally not a general UE4SS SDK. The fork's UEPseudo dependency
// is private, so the bridge declares only the stable loader surface it needs for
// a non-mutating capability probe. Keep the vtable order and HAS_UI data member
// in sync with Yangff/RE-UE4SS commit 9b4552068804a5f5ec2309ca378aeb65af40ee1a.

#include <memory>
#include <string>
#include <string_view>
#include <vector>

namespace RC
{
    using UEStringType = std::u16string;
    using SystemStringViewType = std::string_view;

    namespace GUI
    {
        class GUITab;
    }

    namespace LuaMadeSimple
    {
        class Lua;
    }

    class CppUserModBase
    {
      protected:
        // The installed library was built with HAS_UI. Omitting this member
        // changes every following offset and makes construction unsafe.
        std::vector<std::shared_ptr<GUI::GUITab>> GUITabs{};

      public:
        UEStringType ModName{};
        UEStringType ModVersion{};
        UEStringType ModDescription{};
        UEStringType ModAuthors{};
        UEStringType ModIntendedSDKVersion{};

        CppUserModBase();
        virtual ~CppUserModBase();

        virtual auto on_update() -> void;
        virtual auto on_unreal_init() -> void;
        virtual auto on_ui_init() -> void;
        virtual auto on_program_start() -> void;

        virtual auto on_lua_start(SystemStringViewType,
                                  LuaMadeSimple::Lua&,
                                  LuaMadeSimple::Lua&,
                                  LuaMadeSimple::Lua&,
                                  std::vector<LuaMadeSimple::Lua*>&) -> void;
        virtual auto on_lua_start(LuaMadeSimple::Lua&,
                                  LuaMadeSimple::Lua&,
                                  LuaMadeSimple::Lua&,
                                  std::vector<LuaMadeSimple::Lua*>&) -> void;
        virtual auto on_lua_stop(SystemStringViewType,
                                 LuaMadeSimple::Lua&,
                                 LuaMadeSimple::Lua&,
                                 LuaMadeSimple::Lua&,
                                 std::vector<LuaMadeSimple::Lua*>&) -> void;
        virtual auto on_lua_stop(LuaMadeSimple::Lua&,
                                 LuaMadeSimple::Lua&,
                                 LuaMadeSimple::Lua&,
                                 std::vector<LuaMadeSimple::Lua*>&) -> void;
        virtual auto on_dll_load(SystemStringViewType) -> void;
        virtual auto render_tab() -> void;
    };

    static_assert(sizeof(CppUserModBase) == 0x98,
                  "UE4SS base layout drifted; do not load this bridge");

    namespace Unreal
    {
        class UObject;
        class UClass;

        namespace UObjectGlobals
        {
            auto StaticFindObject_InternalSlow(UClass* object_class,
                                               UObject* object_package,
                                               const char16_t* object_name,
                                               bool exact_class) -> UObject*;
        }
    }
}
