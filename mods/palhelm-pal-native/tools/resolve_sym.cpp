#include <cstdint>
#include <cstring>
#include <fcntl.h>
#include <iostream>
#include <string_view>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>
#include <vector>

#pragma pack(push, 1)
struct Record
{
    std::uint64_t address;
    std::uint32_t line_number;
    std::uint32_t file_relative_offset;
    std::uint32_t symbol_relative_offset;
};
#pragma pack(pop)

static_assert(sizeof(Record) == 20);

auto main(int argc, char** argv) -> int
{
    if (argc < 3)
    {
        std::cerr << "usage: resolve_sym FILE.sym SYMBOL [SYMBOL...]\n";
        return 2;
    }

    const int fd = ::open(argv[1], O_RDONLY);
    if (fd < 0)
    {
        std::cerr << "unable to open symbol file\n";
        return 2;
    }

    struct stat status{};
    if (::fstat(fd, &status) != 0 || status.st_size < 4)
    {
        std::cerr << "invalid symbol file\n";
        ::close(fd);
        return 2;
    }

    const auto size = static_cast<std::size_t>(status.st_size);
    const auto* bytes = static_cast<const std::uint8_t*>(
        ::mmap(nullptr, size, PROT_READ, MAP_PRIVATE, fd, 0));
    ::close(fd);
    if (bytes == MAP_FAILED)
    {
        std::cerr << "unable to map symbol file\n";
        return 2;
    }

    std::uint32_t count{};
    std::memcpy(&count, bytes, sizeof(count));
    const std::size_t strings_offset = sizeof(count) +
                                       static_cast<std::size_t>(count) * sizeof(Record);
    if (strings_offset >= size)
    {
        std::cerr << "invalid record count\n";
        ::munmap(const_cast<std::uint8_t*>(bytes), size);
        return 2;
    }

    const auto* records = reinterpret_cast<const Record*>(bytes + sizeof(count));
    std::vector<bool> found(static_cast<std::size_t>(argc), false);
    for (std::uint32_t index = 0; index < count; ++index)
    {
        const auto relative = records[index].symbol_relative_offset;
        if (relative == UINT32_MAX || relative >= size - strings_offset)
        {
            continue;
        }

        const char* symbol = reinterpret_cast<const char*>(bytes + strings_offset + relative);
        const auto remaining = size - strings_offset - relative;
        const void* newline = std::memchr(symbol, '\n', remaining);
        if (newline == nullptr)
        {
            continue;
        }
        const std::string_view candidate(symbol,
                                         static_cast<const char*>(newline) - symbol);

        for (int argument = 2; argument < argc; ++argument)
        {
            if (candidate == argv[argument])
            {
                std::cout << "0x" << std::hex << records[index].address << std::dec
                          << '\t' << candidate << '\n';
                found[static_cast<std::size_t>(argument)] = true;
                break;
            }
        }
    }

    ::munmap(const_cast<std::uint8_t*>(bytes), size);
    for (int argument = 2; argument < argc; ++argument)
    {
        if (!found[static_cast<std::size_t>(argument)])
        {
            return 1;
        }
    }
    return 0;
}
