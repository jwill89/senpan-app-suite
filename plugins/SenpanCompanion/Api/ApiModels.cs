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

    /// <summary>Spared by "Delete all" (approved custom cards are auto-protected).</summary>
    public bool Protected { get; set; }

    /// <summary>Custom-card lifecycle: "" normal, "pending" awaiting approval, "approved".</summary>
    public string CustomStatus { get; set; } = string.Empty;

    /// <summary>Home world of a custom-card requester ("" for normal cards).</summary>
    public string World { get; set; } = string.Empty;
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

    // Pre-select the auto-draw toggle (Auto) and fill the "Time Between Calls"
    // selector (AutoInterval seconds) when this preset is applied on the Game tab.
    public bool Auto { get; set; }
    public int AutoInterval { get; set; }
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

    // "It's Yoever" reaction: whether it is currently allowed (admin-toggleable
    // per game) and how many times it has fired this game. Delivered both on
    // GET /api/game and on the game_update WebSocket push.
    public bool YoeverEnabled { get; set; }
    public int YoeverCount { get; set; }

    // Automatic-draw state: whether the server is drawing numbers on its own for
    // this game, and the seconds between draws ("Time Between Calls"). Toggleable
    // live; auto switches off at half-time and when a winner is recognized.
    public bool AutoEnabled { get; set; }
    public int AutoInterval { get; set; }
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

// ── Garapon ──────────────────────────────────────────────────────────────────

public sealed class Garapon
{
    public long Id { get; set; }
    public string Title { get; set; } = string.Empty;
    public string Status { get; set; } = string.Empty;
    public long? StampRallyId { get; set; }                     // set when linked to a rally
    public string StampRallyTitle { get; set; } = string.Empty; // joined for display when linked
    public int PlayerCount { get; set; }                        // admin-list aggregate
    public int DrawCount { get; set; }                          // admin-list aggregate
}

// A per-player drawing link. StampCardToken is non-empty only when the garapon is
// tied to a rally and a dual card was auto-issued (it equals Token).
public sealed class GaraponPlayer
{
    public long Id { get; set; }
    public string Token { get; set; } = string.Empty;
    public string PlayerName { get; set; } = string.Empty;
    public int MaxDraws { get; set; }
    public int DrawsUsed { get; set; }
    public string StampCardToken { get; set; } = string.Empty;
}

// One recorded pull (snapshotted, so the log survives later prize edits).
public sealed class GaraponDraw
{
    public string PlayerName { get; set; } = string.Empty;
    public string PrizeName { get; set; } = string.Empty;
    public string BallColor { get; set; } = string.Empty; // CSS hex, e.g. "#e5b53f"
    public string DrawnAt { get; set; } = string.Empty;
}

public sealed class GaraponsResponse
{
    public List<Garapon> Garapons { get; set; } = new();
}

// GET /api/garapons/{id}: the garapon, its drawing links, and the full draw log.
public sealed class GaraponDetailResponse
{
    public Garapon Garapon { get; set; } = new();
    public List<GaraponPlayer> Players { get; set; } = new();
    public List<GaraponDraw> Draws { get; set; } = new();
}

public sealed class GaraponPlayerResponse
{
    public GaraponPlayer Player { get; set; } = new();
}

// ── Stamp Rally ──────────────────────────────────────────────────────────────

// One collectable stamp (a "stall"). AffiliateName "" renders as "Senpan Tea House".
public sealed class StampRallyStamp
{
    public long Id { get; set; }
    public string AffiliateName { get; set; } = string.Empty;
    public bool Paused { get; set; }
    public int SortOrder { get; set; }
}

public sealed class StampRally
{
    public long Id { get; set; }
    public string Title { get; set; } = string.Empty;
    public string Status { get; set; } = string.Empty;
    public int CardCount { get; set; }
    public int CompletedCount { get; set; }
    public int StampCount { get; set; }
    public int ActiveStampCount { get; set; }
    public List<StampRallyStamp> Stamps { get; set; } = new(); // populated on detail fetches
}

// A participant's tokenized card. CollectedCount is an admin-list aggregate.
public sealed class StampRallyCard
{
    public long Id { get; set; }
    public string Token { get; set; } = string.Empty;
    public string ParticipantName { get; set; } = string.Empty;
    public bool Completed { get; set; }
    public int CollectedCount { get; set; }
}

// One row of the event-wide collected-stamp log (grouped by participant server-side).
public sealed class StampRallyLogEntry
{
    public long CardId { get; set; }
    public string ParticipantName { get; set; } = string.Empty;
    public long StampId { get; set; }
    public string StallName { get; set; } = string.Empty;
    public string StampedAt { get; set; } = string.Empty;
}

public sealed class StampRalliesResponse
{
    public List<StampRally> StampRallies { get; set; } = new();
}

// GET /api/stamp-rallies/{id}: the rally (with stamps) plus its issued cards.
public sealed class StampRallyDetailResponse
{
    public StampRally StampRally { get; set; } = new();
    public List<StampRallyCard> Cards { get; set; } = new();
}

public sealed class StampRallyCardResponse
{
    public StampRallyCard Card { get; set; } = new();
}

public sealed class StampRallyLogsResponse
{
    public List<StampRallyLogEntry> Logs { get; set; } = new();
}

// ── Generic ──────────────────────────────────────────────────────────────────

public sealed class OkResponse
{
    public bool Ok { get; set; }
    public bool Deleted { get; set; }
    public string? Error { get; set; }
}

// DeletedCountResponse mirrors the server's bulk-delete envelope — {"deleted": N}.
public sealed class DeletedCountResponse
{
    public long Deleted { get; set; }
}
