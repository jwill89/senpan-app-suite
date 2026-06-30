using System;
using System.Collections.Generic;

namespace SenpanCompanion.Api;

// Data-transfer objects mirroring the Senpan Go server's JSON shapes. The shared
// JsonSerializerOptions (see ApiClient.Json) uses a snake_case naming policy, so
// these PascalCase properties map to the server's snake_case fields without
// per-property attributes (e.g. PlayerName <-> "player_name").

// ── Auth ─────────────────────────────────────────────────────────────────────

public sealed class User
{
    public long Id { get; set; }
    public string Username { get; set; } = string.Empty;
    public bool IsAdmin { get; set; }
    public bool IsActive { get; set; }
    public List<string> Permissions { get; set; } = new();
}

public sealed class AuthCheckResponse
{
    public bool Authenticated { get; set; }
    public User? User { get; set; }
}

// ── Bingo: cards ─────────────────────────────────────────────────────────────

public sealed class CardListEntry
{
    public string Id { get; set; } = string.Empty;
    public string PlayerName { get; set; } = string.Empty;
    public string Details { get; set; } = string.Empty;
    public string CreatedAt { get; set; } = string.Empty;
}

public sealed class CardsResponse
{
    public List<CardListEntry> Cards { get; set; } = new();
}

public sealed class GeneratedCard
{
    public string Id { get; set; } = string.Empty;
    public string PlayerName { get; set; } = string.Empty;
}

public sealed class GenerateSingleResponse
{
    public GeneratedCard Card { get; set; } = new();
    public int Count { get; set; }
}

public sealed class Card
{
    public string Id { get; set; } = string.Empty;
    public int[][] BoardData { get; set; } = Array.Empty<int[]>(); // 5×5; board[row][col], 0 = FREE
    public string PlayerName { get; set; } = string.Empty;
    public string Details { get; set; } = string.Empty;
}

public sealed class BoardResponse
{
    public Card Card { get; set; } = new();
}

// ── Bingo: patterns ──────────────────────────────────────────────────────────

public sealed class Pattern
{
    public long Id { get; set; }
    public string Name { get; set; } = string.Empty;
    public string CategoryName { get; set; } = string.Empty;
    public bool[][] PatternData { get; set; } = Array.Empty<bool[]>(); // 5×5 grid; true = required cell
}

public sealed class PatternsResponse
{
    public List<Pattern> Patterns { get; set; } = new();
}

// ── Bingo: presets ───────────────────────────────────────────────────────────

public sealed class GamePreset
{
    public long Id { get; set; }
    public string Name { get; set; } = string.Empty;
    public List<int> PatternIds { get; set; } = new();
    public string GameDetails { get; set; } = string.Empty;
}

public sealed class PresetsResponse
{
    public List<GamePreset> Presets { get; set; } = new();
}

// ── Bingo: game ──────────────────────────────────────────────────────────────

public sealed class GamePattern
{
    public int Id { get; set; }
    public string Name { get; set; } = string.Empty;
    public bool[][] PatternData { get; set; } = Array.Empty<bool[]>(); // 5×5 snapshot grid
}

public sealed class GameState
{
    public long Id { get; set; }
    public string CreatedAt { get; set; } = string.Empty;
    public List<int> CalledNumbers { get; set; } = new();
    public List<GamePattern> Patterns { get; set; } = new();
    public int TotalCalled { get; set; }
}

public sealed class GameStateResponse
{
    public GameState? Game { get; set; }
    public List<string> Winners { get; set; } = new();
    public string GameDetails { get; set; } = string.Empty;
}

public sealed class DrawnNumber
{
    public int Number { get; set; }
    public string Letter { get; set; } = string.Empty;
    public int CallOrder { get; set; }
}

public sealed class DrawResult
{
    public DrawnNumber Drawn { get; set; } = new();
    public List<string> Winners { get; set; } = new();
}

// ── Raffles ──────────────────────────────────────────────────────────────────

public sealed class Raffle
{
    public long Id { get; set; }
    public string Title { get; set; } = string.Empty;
    public string Status { get; set; } = string.Empty;
    public int MaxEntries { get; set; }
    public double CostPerEntry { get; set; }
    public string SignupInstructions { get; set; } = string.Empty;
}

public sealed class RaffleEntry
{
    public long Id { get; set; }
    public string CharacterName { get; set; } = string.Empty;
    public string World { get; set; } = string.Empty;
    public int NumEntries { get; set; }
    public bool Paid { get; set; }
}

public sealed class RafflesResponse
{
    public List<Raffle> Raffles { get; set; } = new();
}

public sealed class RaffleDetailResponse
{
    public Raffle Raffle { get; set; } = new();
    public int TotalEntries { get; set; }
    public List<RaffleEntry> Entries { get; set; } = new();
}

public sealed class RaffleEntryResponse
{
    public RaffleEntry Entry { get; set; } = new();
}

public sealed class RaffleWinnerResponse
{
    public RaffleEntry? Winner { get; set; }
}

// ── Winners ──────────────────────────────────────────────────────────────────

public sealed class FrequentWinner
{
    public string PlayerName { get; set; } = string.Empty;
    public int WinCount { get; set; }
}

public sealed class FrequentWinnersResponse
{
    public List<FrequentWinner> Winners { get; set; } = new();
}

public sealed class WinnersLogEntry
{
    public long Id { get; set; }
    public string LoggedAt { get; set; } = string.Empty;
    public string CardId { get; set; } = string.Empty;
    public string PlayerName { get; set; } = string.Empty;
    public string GameDetails { get; set; } = string.Empty;
    public string WinningPatterns { get; set; } = string.Empty; // JSON array of pattern names
}

public sealed class WinnersLogResponse
{
    public List<WinnersLogEntry> Entries { get; set; } = new();
    public int Total { get; set; }
    public int Page { get; set; }
    public int PerPage { get; set; }
}

// ── Generic ──────────────────────────────────────────────────────────────────

public sealed class OkResponse
{
    public bool Ok { get; set; }
    public bool Deleted { get; set; }
    public string? Error { get; set; }
}
