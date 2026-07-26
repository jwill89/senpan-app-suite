using System.Collections.Generic;
using System.Globalization;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;

namespace SenpanCompanion.Windows;

/// <summary>
/// Tea Rooms operator panel (Senpan Tea House → Tea Rooms). A compact, at-a-glance
/// availability board: every room listed by number, name, and owner (with its
/// per-half-hour cost), and the two toggles staff flip most often — a room's
/// open/closed status and its 50%-off discount. Everything else about a room
/// (subtitle, image, hashtags, Discord posting, reordering) stays on the website.
///
/// Rooms aren't pushed over the WebSocket, so a Refresh button re-pulls the list.
/// Each toggle hits PATCH /api/tea-rooms/{id} (gated by the teahouse-tea-rooms
/// permission) and swaps the returned room back into the list in place.
/// </summary>
internal sealed class TeaRoomsTab : TabBase
{
    private readonly ApiClient api;

    private List<TeaRoom> rooms = new();

    public TeaRoomsTab(ApiClient api) => this.api = api;

    /// <summary>Reloads the full room list (the server preserves the admin order).</summary>
    protected override async Task LoadAsync()
    {
        var res = await this.api.ListTeaRoomsAsync();
        await Apply(() => this.rooms = res.TeaRooms);
    }

    public void Draw()
    {
        DrawStatusLine();

        if (Ui.Button("Refresh##tearooms"))
            Run(LoadAsync);

        Ui.Section(FontAwesomeIcon.Store, $"Tea Rooms ({this.rooms.Count})");

        if (this.rooms.Count == 0)
        {
            ImGui.TextDisabled(this.Busy ? "Loading…" : "No tea rooms yet.");
            return;
        }

        UiText.WrappedDisabled(
            "Tick a room's Open box to open it, or its Discount box for 50% off. " +
            "Everything else about a room is managed on the website.");

        // A flat table (not a Ui.Box — tables manage their own draw channels). Name and
        // Owner stretch; the number, cost, and the two toggles are fixed. It fills the rest
        // of the content pane and scrolls internally with a pinned header row, so a long
        // room list stays usable, like the other list tabs.
        var height = ImGui.GetContentRegionAvail().Y;
        if (!ImGui.BeginTable("tearooms", 6,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY | ImGuiTableFlags.Resizable,
                new Vector2(0f, height)))
            return;

        ImGui.TableSetupColumn("Room #", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Name");
        ImGui.TableSetupColumn("Owner");
        ImGui.TableSetupColumn("Cost", ImGuiTableColumnFlags.WidthFixed, 160);
        ImGui.TableSetupColumn("Open", ImGuiTableColumnFlags.WidthFixed, 55);
        ImGui.TableSetupColumn("Discount", ImGuiTableColumnFlags.WidthFixed, 80);
        ImGui.TableHeadersRow();

        foreach (var room in this.rooms)
        {
            ImGui.TableNextRow();

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.RoomNumber) ? "—" : room.RoomNumber);

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.Name) ? "—" : room.Name);

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.RoomOwner) ? "—" : room.RoomOwner);

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(CostText(room));

            // Both toggles reflect the room's current state and, on click, PATCH just
            // their own flag. The box reads the model until the server's saved room
            // lands back (ReplaceRoom), matching how the Raffle tab's Paid box behaves.
            ImGui.TableNextColumn();
            var open = room.Open;
            if (ImGui.Checkbox($"##open{room.Id}", ref open))
                SetOpen(room.Id, open);

            ImGui.TableNextColumn();
            var discounted = room.Discounted;
            if (ImGui.Checkbox($"##disc{room.Id}", ref discounted))
                SetDiscounted(room.Id, discounted);
        }

        ImGui.EndTable();
    }

    private void SetOpen(long id, bool open) => Run(async () =>
    {
        var res = await this.api.SetTeaRoomOpenAsync(id, open);
        await Apply(() => ReplaceRoom(res.TeaRoom));
    });

    private void SetDiscounted(long id, bool discounted) => Run(async () =>
    {
        var res = await this.api.SetTeaRoomDiscountedAsync(id, discounted);
        await Apply(() => ReplaceRoom(res.TeaRoom));
    });

    /// <summary>Swaps the server's saved room back into the list in place (matched by id).</summary>
    private void ReplaceRoom(TeaRoom? saved)
    {
        if (saved == null)
            return;
        var i = this.rooms.FindIndex(r => r.Id == saved.Id);
        if (i >= 0)
            this.rooms[i] = saved;
    }

    /// <summary>
    /// The per-half-hour cost, halved with a "(50% off)" note when discounted — the
    /// same fixed 50% rule the website and Discord embed use. Formatted with
    /// invariant thousands separators so it reads the same on any locale.
    /// </summary>
    private static string CostText(TeaRoom room)
    {
        var cost = room.Discounted ? room.CostPerHalfHour / 2 : room.CostPerHalfHour;
        var text = $"{cost.ToString("N0", CultureInfo.InvariantCulture)} gil";
        return room.Discounted ? $"{text} (50% off)" : text;
    }
}
