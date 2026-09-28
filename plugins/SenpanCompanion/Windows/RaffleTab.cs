using System;
using System.Collections.Generic;
using System.Linq;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Raffle operator panel: pick an open raffle, add entrants (with nearby-player
/// quick-fill), toggle paid status, and draw a winner (pick -> confirm, or pick
/// again). Matches the website's admin raffle entry flow; raffle creation stays on
/// the website by design. Raffles aren't broadcast over the WebSocket, so Refresh
/// re-pulls both the list and the open raffle's detail.
/// </summary>
internal sealed class RaffleTab : TabBase
{
    private readonly ApiClient api;
    private readonly NearbyPlayers nearby;

    private List<Raffle> raffles = new();
    private long selectedRaffleId;
    private RaffleDetailResponse? detail;
    private RaffleEntry? pendingWinner;

    private string charName = string.Empty;
    private string world = string.Empty;
    private int numEntries = 1;
    private bool markPaidOnAdd = true;

    // Payment popup form state. One set is enough: only one popup is open at a time,
    // and OpenPaymentPopup reseeds both fields for the entry being settled.
    private int payTickets = 1;
    private int payWaived;

    public RaffleTab(ApiClient api, NearbyPlayers nearby)
    {
        this.api = api;
        this.nearby = nearby;
    }

    /// <summary>Reloads the raffle list and, if one is open, its detail.</summary>
    protected override async Task LoadAsync()
    {
        var selected = this.selectedRaffleId;
        var rafflesRes = await this.api.ListRafflesAsync();
        var detailRes = selected != 0 ? await this.api.GetRaffleAsync(selected) : null;
        await Apply(() =>
        {
            this.raffles = rafflesRes.Raffles;
            // Only if the selection has not moved on while this was in flight.
            if (detailRes != null && this.selectedRaffleId == selected)
                this.detail = detailRes;
        });
    }

    public void Draw()
    {
        DrawStatusLine();

        if (Ui.Button("Refresh##raffles"))
            Run(LoadAsync);
        ImGui.SameLine();
        DrawRafflePicker();

        if (this.detail == null)
        {
            ImGui.TextDisabled("Select a raffle.");
            return;
        }

        ImGui.Separator();
        DrawRaffleHeader(this.detail);

        Ui.Section(FontAwesomeIcon.UserPlus, "Add entrant");
        DrawAddEntry();

        Ui.Section(FontAwesomeIcon.Users, "Entrants");
        DrawEntries(this.detail);

        Ui.Section(FontAwesomeIcon.Trophy, "Draw a winner");
        DrawWinnerControls();
    }

    private void DrawRafflePicker()
    {
        var current = this.raffles.FirstOrDefault(r => r.Id == this.selectedRaffleId);
        var preview = current != null ? $"{current.Title} ({current.Status})" : "Select raffle...";

        ImGui.SetNextItemWidth(280);
        if (!ImGui.BeginCombo("##rafflepick", preview))
            return;
        foreach (var raffle in this.raffles)
        {
            var selected = raffle.Id == this.selectedRaffleId;
            if (ImGui.Selectable($"{raffle.Title} ({raffle.Status})##r{raffle.Id}", selected))
                LoadRaffle(raffle.Id);
        }
        ImGui.EndCombo();
    }

    private void LoadRaffle(long id)
    {
        this.selectedRaffleId = id;
        this.pendingWinner = null;
        // Clear the previous raffle's detail immediately, and only write back a
        // fetch that still matches the selection - the pattern LoadRally and
        // LoadGarapon already use. Without both, picking a raffle while another
        // load was in flight left selectedRaffleId on the NEW raffle while the
        // header, entrant table and winner controls still rendered the OLD one, so
        // "Add entrant" and "Pick a winner" acted on a raffle that was not on
        // screen. TabBase.Run's busy gate silently drops the second fetch, which is
        // what made the mismatch stick.
        this.detail = null;
        Run(async () =>
        {
            var d = await this.api.GetRaffleAsync(id);
            await Apply(() =>
            {
                if (this.selectedRaffleId == id)
                    this.detail = d;
            });
        });
    }

    private static void DrawRaffleHeader(RaffleDetailResponse d)
    {
        ImGui.Text($"{d.Raffle.Title}");
        ImGui.SameLine();
        ImGui.TextDisabled($"- {d.Raffle.Status}, {d.TotalEntries} entr{(d.TotalEntries == 1 ? "y" : "ies")}");

        var costs = DescribeCost(d.Raffle);
        if (costs.Length > 0)
            ImGui.TextDisabled($"{costs}  *  Max per person: {d.Raffle.MaxEntries}");
        else if (!d.Raffle.AcceptsSignups)
            ImGui.TextDisabled($"Details only - players sign up elsewhere  *  Max per person: {d.Raffle.MaxEntries}");
    }

    /// <summary>
    /// The raffle's pricing as one line: a flat cost per entry, or the whole
    /// ladder when each entry has its own price. Empty when nothing is charged.
    /// </summary>
    private static string DescribeCost(Raffle raffle)
    {
        if (string.Equals(raffle.EntryMode, "custom", StringComparison.OrdinalIgnoreCase))
        {
            return raffle.TierCosts.Count == 0
                ? string.Empty
                : "Cost per entry: " + string.Join(" / ", raffle.TierCosts.Select(c => c.ToString("0.##")));
        }

        return raffle.AcceptsSignups && raffle.CostPerEntry > 0
            ? $"Cost per entry: {raffle.CostPerEntry:0.##}"
            : string.Empty;
    }

    private void DrawAddEntry()
    {
        var open = string.Equals(this.detail?.Raffle.Status, "open", StringComparison.OrdinalIgnoreCase);
        if (!open)
        {
            UiText.WrappedDisabled("This raffle is closed - entries can't be added.");
            return;
        }

        // A details-only raffle is published for reference and entered somewhere else
        // entirely, so nobody is signed up through the app. Say why, rather than
        // offering a form that would quietly create an entrant this raffle never
        // meant to collect. (Entrants for one are recorded on the website.)
        if (this.detail != null && !this.detail.Raffle.AcceptsSignups)
        {
            UiText.WrappedDisabled(
                "This raffle is details only - players enter it outside the app, so there is " +
                "no sign-up to take here.");
            return;
        }

        ImGui.SetNextItemWidth(160);
        ImGui.InputText("Name##entry", ref this.charName, 64);
        ImGui.SameLine();
        ImGui.SetNextItemWidth(140);
        ImGui.InputText("World##entry", ref this.world, 32);
        ImGui.SameLine();
        DrawNearbyPicker();

        ImGui.SetNextItemWidth(120);
        if (ImGui.InputInt("Tickets", ref this.numEntries))
            this.numEntries = Math.Max(1, this.numEntries);
        ImGui.SameLine();
        ImGui.Checkbox("Paid", ref this.markPaidOnAdd);
        ImGui.SameLine();

        var canAdd = !string.IsNullOrWhiteSpace(this.charName) && !string.IsNullOrWhiteSpace(this.world);
        if (!canAdd)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton("Add entrant"))
        {
            var id = this.selectedRaffleId;
            var name = this.charName.Trim();
            var w = this.world.Trim();
            var n = Math.Max(1, this.numEntries);
            var paid = this.markPaidOnAdd;
            Run(async () =>
            {
                await this.api.AddRaffleEntryAsync(id, name, w, n, paid);
                var d = await this.api.GetRaffleAsync(id);
                await Apply(() =>
                {
                    this.detail = d;
                    this.charName = string.Empty;
                    this.world = string.Empty;
                    this.numEntries = 1;
                });
            });
        }
        if (!canAdd)
            ImGui.EndDisabled();
    }

    private void DrawEntries(RaffleDetailResponse d)
    {
        if (d.Entries.Count == 0)
        {
            ImGui.TextDisabled("No entries yet.");
            return;
        }

        if (!ImGui.BeginTable("entries", 5,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 200)))
            return;

        ImGui.TableSetupColumn("Name");
        ImGui.TableSetupColumn("World");
        ImGui.TableSetupColumn("Tickets", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Paid", ImGuiTableColumnFlags.WidthFixed, 90);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableHeadersRow();

        foreach (var entry in d.Entries)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.CharacterName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.World);
            ImGui.TableNextColumn();
            // Entries merge per character+world, so an entrant who settled up and
            // then bought more tickets is only PARTLY paid - show both numbers
            // rather than a bare total that hides the outstanding ones.
            if (entry.PartiallyPaid)
            {
                ImGui.TextUnformatted($"{entry.PaidEntries} / {entry.NumEntries}");
                Ui.ItemTooltip($"{entry.PaidEntries} of {entry.NumEntries} tickets settled");
            }
            else
            {
                ImGui.TextUnformatted(entry.NumEntries.ToString());
            }

            ImGui.TableNextColumn();
            DrawPaymentCell(d.Raffle, entry);

            ImGui.TableNextColumn();
            if (Ui.DangerIconButton($"e{entry.Id}", FontAwesomeIcon.Trash, "Delete entrant"))
            {
                var id = this.selectedRaffleId;
                var entryId = entry.Id;
                Run(async () =>
                {
                    await this.api.DeleteRaffleEntryAsync(id, entryId);
                    var d2 = await this.api.GetRaffleAsync(id);
                    await Apply(() => this.detail = d2);
                });
            }
        }

        ImGui.EndTable();
    }

    // -- Payment -------------------------------------------------------------

    /// <summary>
    /// The Paid column: a button showing how far the entry has settled, opening a
    /// popup that records a payment. The popup is where part payments and waived gil
    /// live - both need a number typed, which a checkbox has nowhere to put.
    /// </summary>
    private void DrawPaymentCell(Raffle raffle, RaffleEntry entry)
    {
        var (label, color) = entry.Paid
            ? ("Paid", Ui.SuccessColor)
            : entry.PartiallyPaid
                ? ("Partial", Ui.WarnColor)
                : ("Unpaid", Ui.InfoColor);

        ImGui.PushStyleColor(ImGuiCol.Text, color);
        var open = ImGui.SmallButton($"{label}##pay{entry.Id}");
        ImGui.PopStyleColor();
        Ui.ItemTooltip("Record a payment");
        if (open)
            OpenPaymentPopup(entry);

        DrawPaymentPopup(raffle, entry);
    }

    /// <summary>
    /// Seeds the popup for one entry and opens it. The ticket box starts at
    /// everything outstanding (the common case - somebody paying up in full) and the
    /// waiver starts at zero, never at what was waived before: the server ADDS what
    /// is sent, so re-sending the running total would double it.
    /// </summary>
    private void OpenPaymentPopup(RaffleEntry entry)
    {
        this.payTickets = Math.Max(1, entry.NumEntries);
        this.payWaived = 0;
        ImGui.OpenPopup(PaymentPopupId(entry));
    }

    private static string PaymentPopupId(RaffleEntry entry) => $"##paypopup{entry.Id}";

    private void DrawPaymentPopup(Raffle raffle, RaffleEntry entry)
    {
        if (!ImGui.BeginPopup(PaymentPopupId(entry)))
            return;

        ImGui.TextUnformatted($"{entry.CharacterName} @ {entry.World}");
        Ui.Help($"{entry.NumEntries} ticket{(entry.NumEntries == 1 ? string.Empty : "s")}"
                + (entry.PaidEntries > 0 ? $"  -  {entry.PaidEntries} already settled" : string.Empty));

        var charges = raffle.AcceptsSignups && (raffle.CostPerEntry > 0 || raffle.TierCosts.Count > 0);
        if (charges)
        {
            var outstanding = raffle.EntryCost(entry.NumEntries) - raffle.EntryCost(entry.PaidEntries);
            Ui.Help($"Outstanding: {Math.Max(0, outstanding):0.##}"
                    + (entry.AmountWaived > 0 ? $"  -  {entry.AmountWaived:0.##} waived so far" : string.Empty));
        }

        ImGui.Separator();

        // Part payments: how many of the tickets this settlement covers. Only worth
        // a control when there is more than one left to pay for.
        if (entry.NumEntries > 1)
        {
            ImGui.SetNextItemWidth(120);
            if (ImGui.InputInt("Tickets paid for", ref this.payTickets))
                this.payTickets = Math.Clamp(this.payTickets, 1, entry.NumEntries);
            if (this.payTickets < entry.NumEntries)
                Ui.Help($"Leaves {entry.NumEntries - this.payTickets} outstanding.");
        }

        if (charges)
        {
            ImGui.SetNextItemWidth(140);
            if (ImGui.InputInt("Amount waived", ref this.payWaived, 1000, 10000))
                this.payWaived = Math.Max(0, this.payWaived);
            Ui.Help("Forgiven on this payment only - added to anything waived before.");
        }

        ImGui.Separator();

        if (Ui.PrimaryButton("Mark paid"))
        {
            Settle(entry.Id, true, this.payTickets, this.payWaived);
            ImGui.CloseCurrentPopup();
        }
        if (entry.PaidEntries > 0)
        {
            ImGui.SameLine();
            if (Ui.DangerButton("Clear payment"))
            {
                // Clearing resets the whole row - settled tickets AND waived gil -
                // which is also the only way to walk a settlement back.
                Settle(entry.Id, false, 0, 0);
                ImGui.CloseCurrentPopup();
            }
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel"))
            ImGui.CloseCurrentPopup();

        ImGui.EndPopup();
    }

    /// <summary>Sends one settlement and re-pulls the raffle so the table reflects it.</summary>
    private void Settle(long entryId, bool paid, int tickets, int waived)
    {
        var id = this.selectedRaffleId;
        Run(async () =>
        {
            await this.api.MarkRaffleEntryPaidAsync(id, entryId, paid, tickets, waived);
            var d = await this.api.GetRaffleAsync(id);
            await Apply(() => this.detail = d);
        });
    }

    private void DrawWinnerControls()
    {
        if (this.pendingWinner != null)
        {
            UiText.WrappedColored(new Vector4(0.3f, 0.9f, 0.4f, 1f),
                $"Drawn winner: {this.pendingWinner.CharacterName} @ {this.pendingWinner.World}");

            if (Ui.PrimaryButton("Confirm winner"))
            {
                var id = this.selectedRaffleId;
                Run(async () =>
                {
                    await this.api.VerifyRaffleWinnerAsync(id);
                    var d = await this.api.GetRaffleAsync(id);
                    await Apply(() =>
                    {
                        this.detail = d;
                        this.pendingWinner = null;
                    });
                });
            }
            ImGui.SameLine();
            if (Ui.Button("Draw another"))
            {
                var id = this.selectedRaffleId;
                Run(async () =>
                {
                    var w = (await this.api.PickAnotherRaffleWinnerAsync(id)).Winner;
                    await Apply(() => this.pendingWinner = w);
                });
            }
            return;
        }

        if (Ui.PrimaryButton("Pick a winner"))
        {
            var id = this.selectedRaffleId;
            Run(async () =>
            {
                var w = (await this.api.PickRaffleWinnerAsync(id)).Winner;
                await Apply(() => this.pendingWinner = w);
            });
        }
        ImGui.SameLine();
        // A details-only raffle collects nothing through the app, so every entry is
        // eligible; anything that charges draws on tickets that were settled.
        ImGui.TextDisabled(this.detail?.Raffle.AcceptsSignups == false
            ? "Draws from every entrant."
            : "Draws on paid tickets only.");
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##rafflenearby", "Nearby...", ImGuiComboFlags.NoArrowButton))
            return;
        foreach (var np in this.nearby.Snapshot())
        {
            if (ImGui.Selectable($"{np.Name} ({np.World})"))
            {
                this.charName = np.Name;
                this.world = np.World;
            }
        }
        ImGui.EndCombo();
    }
}
