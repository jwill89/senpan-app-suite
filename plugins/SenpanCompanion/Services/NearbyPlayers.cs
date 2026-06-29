using System.Collections.Generic;
using Dalamud.Game.ClientState.Objects.SubKinds;
using Dalamud.Plugin.Services;

namespace SenpanCompanion.Services;

/// <summary>A nearby character's name + home world, for quick entry into forms.</summary>
public readonly record struct NearbyPlayer(string Name, string World);

/// <summary>
/// Reads visible player characters from the object table so the bingo/raffle forms
/// can offer a "pick nearby" shortcut instead of typing names by hand. Only the
/// name + home world are read — the same fields the player would type to enter —
/// and nothing is collected or sent anywhere until the operator picks someone and
/// submits the form.
/// </summary>
public sealed class NearbyPlayers
{
    private readonly IObjectTable objectTable;

    public NearbyPlayers(IObjectTable objectTable) => this.objectTable = objectTable;

    /// <summary>
    /// Returns the distinct nearby players, sorted by name. Must be called on the
    /// game's framework thread — the window Draw loop already is, so calling it from
    /// UI code is safe.
    /// </summary>
    public List<NearbyPlayer> Snapshot()
    {
        var seen = new HashSet<(string, string)>();
        var list = new List<NearbyPlayer>();

        foreach (var obj in this.objectTable)
        {
            if (obj is not IPlayerCharacter pc)
                continue;

            var name = pc.Name.TextValue;
            if (string.IsNullOrWhiteSpace(name))
                continue;

            var world = pc.HomeWorld.ValueNullable?.Name.ExtractText() ?? string.Empty;
            if (!seen.Add((name, world)))
                continue;

            list.Add(new NearbyPlayer(name, world));
        }

        list.Sort((a, b) => string.CompareOrdinal(a.Name, b.Name));
        return list;
    }
}
