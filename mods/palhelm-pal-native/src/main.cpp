#include "ue4ss_abi.hpp"

#include <array>
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <link.h>
#include <unistd.h>

namespace
{
    constexpr std::array<std::uint8_t, 8> kExpectedGameBuildId{
        0x7f, 0x7e, 0x16, 0x74, 0x07, 0x98, 0x4e, 0xc3,
    };

    auto write_log(const char* message) -> void
    {
        const auto length = std::strlen(message);
        static_cast<void>(::write(STDERR_FILENO, message, length));
    }

    auto align4(std::size_t value) -> std::size_t
    {
        return (value + 3U) & ~std::size_t{3U};
    }

    struct BuildIdSearch
    {
        bool found{};
        bool matches{};
    };

    auto inspect_main_executable(dl_phdr_info* info, std::size_t, void* opaque) -> int
    {
        auto& search = *static_cast<BuildIdSearch*>(opaque);
        if (info->dlpi_name != nullptr && info->dlpi_name[0] != '\0')
        {
            return 0;
        }

        for (std::uint16_t index = 0; index < info->dlpi_phnum; ++index)
        {
            const auto& header = info->dlpi_phdr[index];
            if (header.p_type != PT_NOTE)
            {
                continue;
            }

            const auto* cursor = reinterpret_cast<const std::uint8_t*>(info->dlpi_addr + header.p_vaddr);
            const auto* end = cursor + header.p_memsz;
            while (cursor + sizeof(ElfW(Nhdr)) <= end)
            {
                const auto* note = reinterpret_cast<const ElfW(Nhdr)*>(cursor);
                cursor += sizeof(ElfW(Nhdr));
                const auto* name = cursor;
                cursor += align4(note->n_namesz);
                const auto* description = cursor;
                cursor += align4(note->n_descsz);
                if (cursor > end)
                {
                    break;
                }

                if (note->n_type == NT_GNU_BUILD_ID && note->n_namesz >= 3 &&
                    std::memcmp(name, "GNU", 3) == 0)
                {
                    search.found = true;
                    search.matches = note->n_descsz == kExpectedGameBuildId.size() &&
                                     std::memcmp(description,
                                                 kExpectedGameBuildId.data(),
                                                 kExpectedGameBuildId.size()) == 0;
                    return 1;
                }
            }
        }
        return 1;
    }

    auto game_build_matches() -> bool
    {
        BuildIdSearch search{};
        static_cast<void>(::dl_iterate_phdr(inspect_main_executable, &search));
        return search.found && search.matches;
    }

    auto reflection_surface_present() -> bool
    {
        using RC::Unreal::UObjectGlobals::StaticFindObject_InternalSlow;
        constexpr std::array required_objects{
            u"/Script/Pal.PalUtility:GetInitializedCharacterSaveParemter",
            u"/Script/Pal.PalUtility:PalCaptureSuccess",
            u"/Script/Pal.PalUtility:GetCharacterManager",
            u"/Script/Pal.PalCharacterManager:SpawnNewCharacter",
        };

        for (const auto* object_name : required_objects)
        {
            if (StaticFindObject_InternalSlow(nullptr, nullptr, object_name, false) == nullptr)
            {
                return false;
            }
        }
        return true;
    }

    class PalhelmPalNative final : public RC::CppUserModBase
    {
      public:
        PalhelmPalNative()
        {
            // These stay inside libc++'s short-string representation. The mod is
            // deliberately linked without a second C++ runtime.
            ModName = u"Palhelm";
            ModVersion = u"0.1.0";
            ModAuthors = u"BES Pals";
        }

        auto on_unreal_init() -> void override
        {
            if (!game_build_matches())
            {
                write_log("[PalhelmPalNative] disabled: Palworld build ID mismatch\n");
                return;
            }
            if (!reflection_surface_present())
            {
                write_log("[PalhelmPalNative] disabled: required reflected API missing\n");
                return;
            }

            write_log("[PalhelmPalNative] capability probe ready; mutation is not implemented\n");
        }
    };
}

extern "C" __attribute__((visibility("default"))) auto start_mod() -> RC::CppUserModBase*
{
    return new PalhelmPalNative();
}

extern "C" __attribute__((visibility("default"))) auto uninstall_mod(RC::CppUserModBase* mod) -> void
{
    delete mod;
}
