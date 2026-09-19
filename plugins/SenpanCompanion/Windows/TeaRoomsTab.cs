using System;
using System.Collections.Generic;
using System.Globalization;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Tea Rooms operator panel (Senpan Tea House -> Tea Rooms). A compact, at-a-glance
/// availability board: every room listed by number, name, and owner (with its
/// per-half-hour cost), and the toggles staff flip most often - a room's open/closed
/// status, its 50%-off discount, and its lock. Everything else about a room
/// (subtitle, image, hashtags, Discord posting, reordering) stays on the website.
///
/// A lock either waits on a person, as locks always did, or carries an unlock time
/// the server lifts it at. The expiry arrives back as a `tea_room_unlocked` push,
/// which this panel turns into an in-game alert - so an operator learns a room is
/// free while looking at the game, not at this window.
///
/// Rooms aren't otherwise pushed over the WebSocket, so a Refresh button re-pulls
/// the list. Each toggle hits PATCH /api/tea-rooms/{id} (gated by the
/// teahouse-tea-rooms permission) and swaps the returned room back into the list in
/// place.
/// </summary>
internal sealed class TeaRoomsTab : TabBase, IDisposable
{
    /// <summary>Accepted spellings of a typed unlock time (local, 24-hour).</summary>
    private static readonly string[] LockTimeFormats =
    {
        "yyyy-MM-dd HH:mm", "yyyy-MM-dd H:mm", "yyyy-MM-ddTHH:mm", "yyyy-MM-dd HH:mm:ss",
    };

    /// <summary>Quick "lock for a while" offsets, the usual lengths of a booking.</summary>
    private static readonly (string Label, TimeSpan Span)[] LockPresets =
    {
        ("+30m", TimeSpan.FromMinutes(30)),
        ("+1h", TimeSpan.FromHours(1)),
        ("+2h", TimeSpan.FromHours(2)),
        ("+4h", TimeSpan.FromHours(4)),
    };

    private readonly ApiClient api;
    private readonly LiveConnection live;

    private List<TeaRoom> rooms = new();

    // The lock popup's staged unlock time (local wall clock, blank = no expiry) and
    // its inline complaint. Only one popup is open at a time, so one field does.
    private string lockUntil = string.Empty;
    private string lockError = string.Empty;

    public TeaRoomsTab(ApiClient api, LiveConnection live)
    {
        this.api = api;
        this.live = live;
        this.live.TeaRoomUnlocked += OnTeaRoomUnlocked;
        this.live.Reconnected += OnReconnected;
    }

    public void Dispose()
    {
        this.live.TeaRoomUnlocked -= OnTeaRoomUnlocked;
        this.live.Reconnected -= OnReconnected;
    }

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
            ImGui.TextDisabled(this.Busy ? "Loading..." : "No tea rooms yet.");
            return;
        }

        UiText.WrappedDisabled(
            "Tick a room's Open box to open it, or its Discount box for 50% off. Lock takes an " +
            "optional unlock time - leave it blank and the room stays locked until someone unlocks " +
            "it. Everything else about a room is managed on the website.");

        // A flat table (not a Ui.Box - tables manage their own draw channels). Name and
        // Owner stretch; the number, cost, and the toggles are fixed. It fills the rest
        // of the content pane and scrolls internally with a pinned header row, so a long
        // room list stays usable, like the other list tabs.
        var height = ImGui.GetContentRegionAvail().Y;
        if (!ImGui.BeginTable("tearooms", 7,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY | ImGuiTableFlags.Resizable,
                new Vector2(0f, height)))
            return;

        ImGui.TableSetupColumn("Room #", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Name");
        ImGui.TableSetupColumn("Owner");
        ImGui.TableSetupColumn("Cost", ImGuiTableColumnFlags.WidthFixed, 160);
        ImGui.TableSetupColumn("Open", ImGuiTableColumnFlags.WidthFixed, 55);
        ImGui.TableSetupColumn("Discount", ImGuiTableColumnFlags.WidthFixed, 80);
        ImGui.TableSetupColumn("Lock", ImGuiTableColumnFlags.WidthFixed, 210);
        ImGui.TableHeadersRow();

        foreach (var room in this.rooms)
        {
            ImGui.TableNextRow();

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.RoomNumber) ? "-" : room.RoomNumber);

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.Name) ? "-" : room.Name);

            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(room.RoomOwner) ? "-" : room.RoomOwner);

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

            ImGui.TableNextColumn();
            DrawLockCell(room);
        }

        ImGui.EndTable();
    }

    /// <summary>
    /// The Lock column: a button that unlocks straight away, or opens the popup where
    /// the unlock time is chosen. Locking goes through a popup rather than a checkbox
    /// because the time needs typing, and a checkbox has nowhere to put it - the same
    /// reason the Raffle tab's Paid cell is a popup.
    /// </summary>
    private void DrawLockCell(TeaRoom room)
    {
        if (room.Locked)
        {
            if (ImGui.SmallButton($"Unlock##lock{room.Id}"))
                SetLock(room.Id, false, string.Empty);
            Ui.ItemTooltip("Unlock this room now");
            ImGui.SameLine();
            ImGui.TextDisabled(LockText(room));
            return;
        }

        if (ImGui.SmallButton($"Lock##lock{room.Id}"))
        {
            this.lockUntil = string.Empty;
            this.lockError = string.Empty;
            ImGui.OpenPopup(LockPopupId(room));
        }
        Ui.ItemTooltip("Lock this room, with or without an unlock time");
        DrawLockPopup(room);
    }

    private static string LockPopupId(TeaRoom room) => $"##lockpopup{room.Id}";

    private void DrawLockPopup(TeaRoom room)
    {
        if (!ImGui.BeginPopup(LockPopupId(room)))
            return;

        ImGui.TextUnformatted($"Lock {room.Name}");
        Ui.Help("Unlock time (local, e.g. 2026-09-05 21:30). Leave blank to keep the room "
                + "locked until someone unlocks it.");

        ImGui.SetNextItemWidth(200);
        ImGui.InputTextWithHint($"##lockuntil{room.Id}", "yyyy-MM-dd HH:mm", ref this.lockUntil, 32);

        // Typing a full timestamp for "half an hour from now" is the common case, so
        // offer it as a button that fills the field rather than replacing it - the
        // operator still sees, and can correct, the moment they are committing to.
        foreach (var (label, span) in LockPresets)
        {
            if (ImGui.SmallButton($"{label}##lockpreset{room.Id}"))
            {
                this.lockUntil = DateTime.Now.Add(span).ToString("yyyy-MM-dd HH:mm", CultureInfo.InvariantCulture);
                this.lockError = string.Empty;
            }
            ImGui.SameLine();
        }
        if (ImGui.SmallButton($"Clear##lockclear{room.Id}"))
        {
            this.lockUntil = string.Empty;
            this.lockError = string.Empty;
        }

        if (!string.IsNullOrEmpty(this.lockError))
            UiText.WrappedColored(Ui.WarnColor, this.lockError);

        ImGui.Separator();

        if (Ui.PrimaryButton($"Lock room##lockgo{room.Id}"))
        {
            if (TryBuildLockUntil(out var until, out var error))
            {
                SetLock(room.Id, true, until);
                ImGui.CloseCurrentPopup();
            }
            else
            {
                this.lockError = error;
            }
        }
        ImGui.SameLine();
        if (Ui.Button($"Cancel##lockcancel{room.Id}"))
            ImGui.CloseCurrentPopup();

        ImGui.EndPopup();
    }

    /// <summary>
    /// Turns the typed local time into the UTC instant the server stores, or reports
    /// why it can't. A blank field is valid and means no expiry. A moment already gone
    /// is refused here: the server would take it, expire the lock on its next sweep
    /// and alert everyone about a room that was never really locked.
    /// </summary>
    private bool TryBuildLockUntil(out string until, out string error)
    {
        until = string.Empty;
        error = string.Empty;

        var text = this.lockUntil.Trim();
        if (text.Length == 0)
            return true;

        if (!DateTime.TryParseExact(text, LockTimeFormats, CultureInfo.InvariantCulture,
                DateTimeStyles.None, out var local))
        {
            error = "Use a date and time like 2026-09-05 21:30.";
            return false;
        }
        if (local <= DateTime.Now)
        {
            error = "That time has already passed.";
            return false;
        }

        until = DateTime.SpecifyKind(local, DateTimeKind.Local).ToUniversalTime()
            .ToString("yyyy-MM-ddTHH:mm:ssZ", CultureInfo.InvariantCulture);
        return true;
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

    private void SetLock(long id, bool locked, string until) => Run(async () =>
    {
        var res = await this.api.SetTeaRoomLockAsync(id, locked, until);
        await Apply(() => ReplaceRoom(res.TeaRoom));
    });

    /// <summary>
    /// A room's lock ran out and the server lifted it. Correct the row we are holding
    /// (this is the one room change that arrives without anyone asking for it), then
    /// say so in game.
    /// </summary>
    private void OnTeaRoomUnlocked(long id, string name, string roomNumber)
    {
        var i = this.rooms.FindIndex(r => r.Id == id);
        if (i >= 0)
        {
            this.rooms[i].Locked = false;
            this.rooms[i].LockedUntil = string.Empty;
        }
        AlertUnlocked(name, roomNumber);
    }

    /// <summary>
    /// Tells the operator a room's lock has expired, wherever they are - the point of
    /// an unlock time is that nobody has to watch for it.
    /// </summary>
    /// <remarks>
    /// All three channels are purely LOCAL. IChatGui.Print writes into this client's
    /// own chat log and sends nothing to the server - it is not ChatSender, which
    /// actually transmits and carries the account risk that comes with automated chat.
    /// The toast is Dalamud's own overlay, and the chime is synthesized audio played
    /// through winmm with no game interop at all (see Services/Chime). So this is a
    /// notice only the operator can see or hear, about a change they asked the server
    /// to make.
    /// </remarks>
    private static void AlertUnlocked(string name, string roomNumber)
    {
        var room = string.IsNullOrWhiteSpace(name) ? "A tea room" : name;
        var full = string.IsNullOrWhiteSpace(roomNumber) ? room : $"{room} (Room {roomNumber})";

        Chime.Unlock();
        Plugin.ToastGui.ShowNormal($"{room} is unlocked");
        Plugin.ChatGui.Print($"[Senpan] Room lock expired - {full} is now unlocked and available.");
    }

    /// <summary>
    /// The socket came back after a drop. Locks may have expired while it was down -
    /// the pushes that said so are gone - so the list on screen can claim a room is
    /// still locked when it isn't. Mark it stale and let the per-frame EnsureLoaded
    /// re-pull it, as the bingo page does.
    /// </summary>
    private void OnReconnected() => MarkStale();

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
    /// The per-half-hour cost, halved with a "(50% off)" note when discounted - the
    /// same fixed 50% rule the website and Discord embed use. Formatted with
    /// invariant thousands separators so it reads the same on any locale.
    /// </summary>
    private static string CostText(TeaRoom room)
    {
        var cost = room.Discounted ? room.CostPerHalfHour / 2 : room.CostPerHalfHour;
        var text = $"{cost.ToString("N0", CultureInfo.InvariantCulture)} gil";
        return room.Discounted ? $"{text} (50% off)" : text;
    }

    /// <summary>
    /// When a locked room frees up, in the operator's own timezone - or that it is
    /// waiting on them. The stored value is a UTC instant.
    /// </summary>
    private static string LockText(TeaRoom room)
    {
        if (string.IsNullOrWhiteSpace(room.LockedUntil))
            return "until unlocked";
        return DateTimeOffset.TryParse(room.LockedUntil, CultureInfo.InvariantCulture,
                   DateTimeStyles.AssumeUniversal, out var at)
            ? $"until {at.ToLocalTime().ToString("MMM d HH:mm", CultureInfo.InvariantCulture)}"
            : $"until {room.LockedUntil}";
    }
}
