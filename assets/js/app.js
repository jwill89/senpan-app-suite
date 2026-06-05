/**
 * App Suite — Vue 3 Single-Page Application (CDN, no build step).
 *
 * This file contains the entire client-side application logic for both the
 * player bingo board and the admin dashboard. It communicates with the Go
 * backend via REST API calls and a WebSocket connection for real-time updates.
 *
 * Architecture:
 * - Views are controlled by `this.view`: "home", "admin-login", "admin", "player"
 * - Admin sub-navigation uses `this.adminSection` ("bingo", "raffles", "system")
 *   and `this.adminTab` (e.g. "bingo-game", "bingo-cards", "bingo-patterns", etc.)
 * - Player state (stamps) is persisted in localStorage keyed by card+game ID
 * - WebSocket provides real-time pushes for draws, game state, patterns, cards, and styles
 * - Polling is used only as a fallback when WebSocket is not connected
 *
 * @see AGENTS.md for full architecture documentation
 */
const API = 'api';

const {createApp} = Vue;

createApp({
    data() {
        return {
            view: 'home',
            toast: {show: false, message: '', type: 'info'},
            toastTimer: null,

            // Player
            joinId: '',
            joinError: '',
            playerCard: null,
            playerGame: null,
            stamps: {},       // { "r-c": true }
            stampShape: localStorage.getItem('bingo_stamp_shape') || 'blank',
            customStampImage: null, // data URL for user-uploaded custom stamp (session-only, not persisted)
            stampShapes: [
                {id: 'blank', emoji: '', name: 'Blank'},
                {id: 'heart', emoji: '♥️', name: 'Heart'},
                {id: 'star', emoji: '⭐', name: 'Star'},
                {id: 'smiley', emoji: '😊', name: 'Smiley'},
                {id: 'upside-down-face', emoji: '🙃', name: 'Upside-Down Face'},
                {id: 'expressionless', emoji: '😑', name: 'Expressionless'},
                {id: 'crying', emoji: '😭', name: 'Crying'},
                {id: 'skull', emoji: '💀', name: 'Skull'},
            ],
            stampColor: localStorage.getItem('bingo_stamp_color') || 'pink',
            stampColors: [
                {id: 'pink', name: 'Pink', value: 'rgba(229,49,112,.55)'},
                {id: 'red', name: 'Red', value: 'rgba(255,0,0,.55)'},
                {id: 'orange', name: 'Orange', value: 'rgba(255,152,0,.55)'},
                {id: 'gold', name: 'Gold', value: 'rgba(255,216,3,.55)'},
                {id: 'green', name: 'Green', value: 'rgba(44,182,125,.55)'},
                {id: 'blue', name: 'Blue', value: 'rgba(56,128,255,.55)'},
                {id: 'purple', name: 'Purple', value: 'rgba(127,90,240,.55)'},
            ],
            stampOpacity: parseFloat(localStorage.getItem('bingo_stamp_opacity')) || 0.8,

            // Admin auth
            adminPassword: '',
            authError: '',

            // Admin state
            adminTab: 'bingo-game',
            adminSection: 'bingo', // bingo, raffles, system
            cards: [],
            patterns: [],
            generateCount: 10,
            previewCard: null,

            // Pattern editor
            newPatternName: '',
            newPatternGrid: this.emptyGrid(),
            newPatternCategoryId: null, // null = first category

            // Collapse state for patterns
            patternsCollapsed: false,
            collapsedCategories: {},

            // Pattern categories
            categories: [],
            newCategoryName: '',
            editingCategoryId: null,
            editingCategoryName: '',

            // Pattern inline editing
            editingPatternId: null,
            editingPatternName: '',

            // Drag & drop placeholders
            categoryDragPlaceholder: null, // { beforeId: catId } or { afterId: catId }
            patternDragPlaceholder: null,  // { categoryId, beforeId } or { categoryId, afterId }

            // Game start — filtering
            patternCategoryFilter: null, // null = all categories
            patternSearchQuery: '',

            // Game
            currentGame: null,
            winners: [],
            selectedPatternIds: [],
            lastDrawn: null,
            gameDetails: '',
            drawDelay: 0,           // seconds to delay player reveal
            drawCountdown: null,    // countdown seconds remaining (null = not counting)
            drawSent: false,        // true briefly after number sent to players
            _drawCountdownTimer: null,

            // Winner verification modal
            winnerPreview: null, // { card, matchedCells: Set of "r-c" }
            winnerLoading: false,
            previewLoading: false,

            // Halftime minigame modals
            showHalftimePrompt: false,  // admin: "do you want to alert users?"
            showMinigameModal: false,   // player: "halftime minigame!" alert

            // WebSocket
            _ws: null,
            _wsReconnectTimer: null,
            _wsReconnectAttempts: 0,
            _wsMaxReconnect: 10,

            // Styles
            styles: [],
            editingStyle: null,
            activeStyleId: '',
            _cmEditor: null,

            // App settings (loaded from /api/settings)
            appSettings: {
                app_title: 'Senpan App Suite',
                default_draw_delay: '0',
                frequent_winner_threshold: '3',
                frequent_winner_hours: '12',
                header_font: 'Arapey',
                google_fonts_api_key: '',
            },
            googleFontsList: [],
            _googleFontsCacheKey: '',
            fontPreviewText: 'BINGO 1 2 3',

            // Raffles
            homeRaffles: [],          // open raffles loaded on mount for home card visibility
            raffles: [],
            selectedRaffle: null,
            raffleEntries: [],
            raffleForm: null,        // admin create/edit form
            raffleSignup: {characterName: '', world: '', numEntries: 1},
            raffleSignupResult: null, // result after signing up
            raffleWinner: null,       // picked winner entry
            raffleWinnerEntry: null,  // winner entry for public closed raffle view
            raffleTotalEntryCount: 0, // total entries for public closed raffle view
            raffleImageUploading: false,

            // Winners log
            winnersLog: [],
            winnersLogTotal: 0,
            winnersLogPage: 1,
            winnersLogPerPage: 25,
            winnersLogSort: 'logged_at',
            winnersLogDir: 'desc',

            // Frequent winners (admin game screen)
            frequentWinners: [],

            // End game winner confirmation
            showEndGameModal: false,
            endGameSelectedWinners: [],

            // Card preview inline editing
            previewCardEditing: null,
            previewCardEditValue: '',
            cardSearchQuery: '',
        };
    },

    computed: {
        /** Set of called numbers for the player view (for O(1) lookup in templates). */
        playerCalledSet() {
            if (!this.playerGame || !this.playerGame.called_numbers) return new Set();
            return new Set(this.playerGame.called_numbers);
        },
        /** Set of called numbers for the admin view (for O(1) lookup in templates). */
        adminCalledSet() {
            if (!this.currentGame || !this.currentGame.called_numbers) return new Set();
            return new Set(this.currentGame.called_numbers);
        },
        /** Returns the emoji character for the currently selected stamp shape. */
        currentStampEmoji() {
            const found = this.stampShapes.find(s => s.id === this.stampShape);
            return found ? found.emoji : '';
        },
        /** Returns the RGBA background color string for the current stamp color. */
        currentStampBg() {
            const found = this.stampColors.find(c => c.id === this.stampColor);
            return found ? found.value : 'rgba(229,49,112,.55)';
        },
        /** Inline style object applied to stamp marks (background + opacity). */
        stampMarkStyle() {
            return {background: this.currentStampBg, opacity: this.stampOpacity};
        },
        /** Renders gameDetails as HTML via marked.js (with fallback for plain text). */
        renderedGameDetails() {
            if (!this.gameDetails) return '';
            if (typeof marked !== 'undefined' && marked.parse) {
                return marked.parse(this.gameDetails, {breaks: true});
            }
            // Fallback: escape HTML and convert newlines to <br>
            return this.gameDetails
                .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
                .replace(/\n/g, '<br>');
        },

        /**
         * Patterns filtered by the selected category and search query,
         * used in the game-start pattern picker.
         */
        gameFilteredPatterns() {
            let list = this.patterns;
            if (this.patternCategoryFilter) {
                list = list.filter(p => p.category_id === this.patternCategoryFilter);
            }
            const q = (this.patternSearchQuery || '').trim().toLowerCase();
            if (q) {
                list = list.filter(p => p.name.toLowerCase().includes(q));
            }
            return list;
        },

        /** Returns only open raffles from the loaded raffles list. */
        openRaffles() {
            return this.raffles.filter(r => r.status === 'open');
        },
        /** Returns only closed raffles from the loaded raffles list. */
        closedRaffles() {
            return this.raffles.filter(r => r.status === 'closed');
        },

        /** Dynamic label for the admin game tab: shows "Current Game" or "New Game". */
        adminGameLabel() {
            return this.currentGame ? 'Current Game' : 'New Game';
        },

        /** Filters cards by search query matching ID or player name. */
        filteredCards() {
            const q = this.cardSearchQuery.trim().toLowerCase();
            if (!q) return this.cards;
            return this.cards.filter(c =>
                c.id.toLowerCase().includes(q) ||
                (c.player_name && c.player_name.toLowerCase().includes(q))
            );
        },

        /**
         * Patterns grouped by category for the Patterns admin tab.
         */
        patternsByCategory() {
            const map = {};
            for (const cat of this.categories) {
                map[cat.id] = {category: cat, patterns: []};
            }
            for (const p of this.patterns) {
                if (!map[p.category_id]) {
                    map[p.category_id] = {category: {id: p.category_id, name: p.category_name || 'Unknown'}, patterns: []};
                }
                map[p.category_id].patterns.push(p);
            }
            // Return in category sort order
            return this.categories.map(c => map[c.id]).filter(Boolean);
        },
    },

    methods: {
        /* ====== Helpers ====== */

        /** Creates an empty 5×5 boolean grid for the pattern editor. */
        emptyGrid() {
            return Array.from({length: 5}, () => Array.from({length: 5}, () => false));
        },

        /** Returns the 15 bingo numbers for a given column index (0=B, 1=I, 2=N, 3=G, 4=O). */
        columnNumbers(colIndex) {
            const start = colIndex * 15 + 1;
            return Array.from({length: 15}, (_, i) => start + i);
        },

        /** Checks if a cell at [row, col] has been stamped by the player. */
        isStamped(ri, ci) {
            return !!this.stamps[ri + '-' + ci];
        },

        /** Persists the selected stamp shape to localStorage. */
        setStampShape(id) {
            this.stampShape = id;
            localStorage.setItem('bingo_stamp_shape', id);
        },

        /**
         * Handles custom stamp image upload from a file input.
         * Reads the selected image as a data URL stored in memory (session-only).
         * Warns the user if the image is not square. Only one custom image at a time.
         * Supports transparent formats (PNG, GIF, WebP).
         * @param {Event} event - The file input change event.
         */
        uploadCustomStamp(event) {
            const file = event.target.files && event.target.files[0];
            if (!file) return;
            if (!file.type.startsWith('image/')) {
                this.showToast('Please select an image file.', 'error');
                event.target.value = '';
                return;
            }
            const reader = new FileReader();
            reader.onload = (e) => {
                const dataUrl = e.target.result;
                // Check dimensions and warn if not square
                const img = new Image();
                img.onload = () => {
                    if (img.width !== img.height) {
                        this.showToast(`Image is ${img.width}×${img.height}. Square images work best — non-square images will be stretched to fit.`, 'error');
                    }
                    this.customStampImage = dataUrl;
                    this.setStampShape('custom');
                };
                img.onerror = () => {
                    this.showToast('Could not load image.', 'error');
                };
                img.src = dataUrl;
            };
            reader.readAsDataURL(file);
            event.target.value = ''; // reset so re-uploading the same file triggers change
        },

        /** Persists the selected stamp color to localStorage. */
        setStampColor(id) {
            this.stampColor = id;
            localStorage.setItem('bingo_stamp_color', id);
        },

        /** Persists the selected stamp opacity to localStorage. */
        setStampOpacity(val) {
            this.stampOpacity = parseFloat(val);
            localStorage.setItem('bingo_stamp_opacity', val);
        },

        /** Returns whether a number has been called in the current player game. */
        isCalledPlayer(n) {
            return this.playerCalledSet.has(n);
        },
        /** Returns whether a number has been called in the current admin game. */
        isCalledAdmin(n) {
            return this.adminCalledSet.has(n);
        },

        /** Builds CSS class array for a board cell based on stamp state and FREE status. */
        boardCellClass(ri, ci, cell, isAdmin) {
            const classes = ['board-cell'];
            if (cell === 0) classes.push('free');
            const key = ri + '-' + ci;
            if (this.stamps[key]) classes.push('stamped');
            return classes;
        },

        /** Toggles the stamp on/off for a board cell; persists to localStorage. */
        toggleStamp(ri, ci, cell) {
            const key = ri + '-' + ci;
            if (this.stamps[key]) {
                delete this.stamps[key];
            } else {
                this.stamps[key] = true;
            }
            // Force reactivity
            this.stamps = {...this.stamps};
            this.saveStamps();
        },

        /** Removes all stamps from the player's board. */
        clearAllStamps() {
            this.stamps = {};
            this.saveStamps();
        },

        /** Persists current stamps to localStorage, keyed by card ID + game ID. */
        saveStamps() {
            if (!this.playerCard || !this.playerGame) return;
            const k = 'stamps_' + this.playerCard.id + '_' + this.playerGame.id;
            localStorage.setItem(k, JSON.stringify(this.stamps));
        },

        /** Loads stamps from localStorage for the current card + game combination. */
        loadStamps() {
            if (!this.playerCard || !this.playerGame) {
                this.stamps = {};
                return;
            }
            const k = 'stamps_' + this.playerCard.id + '_' + this.playerGame.id;
            const raw = localStorage.getItem(k);
            this.stamps = raw ? JSON.parse(raw) : {};
        },

        /** Displays a toast notification that auto-dismisses after 3.5 seconds. */
        notify(msg, type = 'info') {
            clearTimeout(this.toastTimer);
            this.toast = {show: true, message: msg, type};
            this.toastTimer = setTimeout(() => {
                this.toast.show = false;
            }, 3500);
        },

        /** Copies text to the clipboard and shows a toast notification. */
        copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(() => {
                this.notify('Copied to clipboard!', 'success');
            }).catch(() => {
                this.notify('Failed to copy', 'error');
            });
        },

        /**
         * Makes an API call to the backend. Automatically handles JSON serialization,
         * credentials (cookies), and error extraction from response body.
         * @param {string} endpoint - API path relative to the API base (e.g. "game", "cards")
         * @param {RequestInit} options - Fetch options; body objects are auto-stringified
         * @returns {Promise<object>} Parsed JSON response
         * @throws {Error} With the server's error message if response is not OK
         */
        async api(endpoint, options = {}) {
            const url = API + '/' + endpoint;
            const opts = {credentials: 'include', ...options};
            if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData)) {
                opts.headers = {'Content-Type': 'application/json', ...(options.headers || {})};
                opts.body = JSON.stringify(options.body);
            }
            const res = await fetch(url, opts);
            const data = await res.json();
            if (!res.ok) {
                throw new Error(data.error || 'Request failed');
            }
            return data;
        },

        /* ====== WebSocket ====== */

        /**
         * Build the WebSocket URL from the current page location.
         * Supports both ws:// and wss:// depending on the page protocol.
         */
        _wsUrl() {
            const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
            let url = proto + '//' + location.host + '/' + API + '/ws';
            if (this.playerCard && this.playerCard.id) {
                url += '?id=' + encodeURIComponent(this.playerCard.id);
            }
            return url;
        },

        /**
         * Open (or re-open) the shared WebSocket connection.
         * Incoming `game_update` messages update both the player and admin state.
         */
        connectWS() {
            const attempts = this._wsReconnectAttempts;
            this.disconnectWS();
            this._wsReconnectAttempts = attempts;

            const isReconnect = this._wsReconnectAttempts > 0;
            const ws = new WebSocket(this._wsUrl());

            ws.onopen = () => {
                // Clear any pending reconnect attempt
                clearTimeout(this._wsReconnectTimer);
                this._wsReconnectTimer = null;

                // If this was a reconnect, notify the user and catch up on missed state
                if (isReconnect) {
                    this.notify('Reconnected!', 'success');
                    this._refreshStateAfterReconnect();
                }
                this._wsReconnectAttempts = 0;
            };

            ws.onmessage = (evt) => {
                let msg;
                try { msg = JSON.parse(evt.data); } catch { return; }

                switch (msg.type) {
                    case 'game_update':
                        this._handleGameUpdate(msg);
                        break;
                    case 'game_draw':
                        this._handleGameDraw(msg);
                        break;
                    case 'cards_update':
                        this._handleCardsUpdate(msg);
                        break;
                    case 'patterns_update':
                        this._handlePatternsUpdate(msg);
                        break;
                    case 'card_deleted':
                        this._handleCardDeleted();
                        break;
                    case 'details_update':
                        this.gameDetails = msg.game_details || '';
                        break;
                    case 'style_update':
                        this._applyCustomCSS(msg.css || '');
                        break;
                    case 'settings_update':
                        if (msg.app_title) {
                            this.appSettings.app_title = msg.app_title;
                            document.title = msg.app_title;
                        }
                        if (msg.header_font) {
                            this.appSettings.header_font = msg.header_font;
                            this._applyHeaderFont(msg.header_font);
                        }
                        break;
                    case 'halftime_minigame':
                        if (this.view === 'player') {
                            this.showMinigameModal = true;
                        }
                        break;
                }
            };

            ws.onclose = () => {
                this._ws = null;
                // Reconnect if we're still on a view that needs it
                if (this.view === 'player' || this.view === 'admin') {
                    this._scheduleReconnect();
                }
            };

            ws.onerror = () => {
                // onclose will fire after onerror — reconnect is handled there
            };

            this._ws = ws;

            // Client-side keepalive: send a small message every 25s to prevent
            // reverse-proxy idle timeouts from killing the connection.
            clearInterval(this._wsKeepalive);
            this._wsKeepalive = setInterval(() => {
                if (this._ws && this._ws.readyState === WebSocket.OPEN) {
                    this._ws.send('ping');
                }
            }, 25000);
        },

        /** Closes the WebSocket connection and cancels any pending reconnect timers. */
        disconnectWS() {
            clearTimeout(this._wsReconnectTimer);
            this._wsReconnectTimer = null;
            this._wsReconnectAttempts = 0;
            clearInterval(this._wsKeepalive);
            this._wsKeepalive = null;
            if (this._ws) {
                this._ws.onclose = null; // prevent reconnect loop
                this._ws.close();
                this._ws = null;
            }
        },

        /**
         * Schedules a WebSocket reconnect with exponential back-off.
         * Gives up after _wsMaxReconnect attempts and notifies the user to refresh.
         */
        _scheduleReconnect() {
            if (this._wsReconnectTimer) return;

            this._wsReconnectAttempts++;

            if (this._wsReconnectAttempts > this._wsMaxReconnect) {
                this.notify('Connection lost. Please refresh the page.', 'error');
                this._wsReconnectAttempts = 0;
                return;
            }

            // Exponential back-off: 1s, 2s, 4s, 8s, 16s (capped at 16s)
            const delay = Math.min(1000 * Math.pow(2, this._wsReconnectAttempts - 1), 16000);
            this.notify(
                `Connection lost. Reconnecting (${this._wsReconnectAttempts}/${this._wsMaxReconnect})…`,
                'info'
            );

            this._wsReconnectTimer = setTimeout(() => {
                this._wsReconnectTimer = null;
                this.connectWS();
            }, delay);
        },

        /**
         * After a successful WebSocket reconnect, fetch the latest state
         * via the REST API to catch up on any updates missed while disconnected.
         */
        async _refreshStateAfterReconnect() {
            try {
                if (this.view === 'player' && this.playerCard) {
                    const data = await this.api('board?id=' + encodeURIComponent(this.playerCard.id));
                    this.playerGame = data.game;
                    this.gameDetails = data.game_details || '';
                } else if (this.view === 'admin') {
                    await this.loadGameState();
                }
            } catch (e) {
                // Silent — WebSocket will deliver future updates
            }
        },

        /**
         * Handle a game_update message (game start or end).
         * Players receive game state + details on start; admins receive game state + winners.
         * On end, both receive game=null.
         */
        _handleGameUpdate(msg) {
            const game = msg.game;

            // Update game details if included (players get this on game start)
            if (msg.game_details !== undefined) {
                this.gameDetails = msg.game_details;
            }

            // Update player view
            if (this.view === 'player' && this.playerCard) {
                const oldGameId = this.playerGame?.id;
                this.playerGame = game;
                // If game changed (new game started), reload stamps
                if (game && game.id !== oldGameId) {
                    this.loadStamps();
                }
                // Clear details when game ends for players
                if (!game) {
                    this.gameDetails = '';
                }
            }

            // Update admin view
            if (this.view === 'admin') {
                this.currentGame = game;
                this.winners = msg.winners || [];
                if (!game) {
                    // Game ended
                    this.lastDrawn = null;
                    this.winnerPreview = null;
                }
            }
        },

        /**
         * Handle a game_draw message.
         * Players receive only the drawn number (appended to local state).
         * Admins receive the drawn number + updated winners list.
         */
        _handleGameDraw(msg) {
            const drawn = msg.drawn;
            if (!drawn) return;

            // Update player view — append the drawn number to local called_numbers
            if (this.view === 'player' && this.playerGame) {
                if (!this.playerGame.called_numbers) {
                    this.playerGame.called_numbers = [];
                }
                this.playerGame.called_numbers.push(drawn.number);
                this.playerGame.total_called = this.playerGame.called_numbers.length;
            }

            // Update admin view — only if we don't already have this number
            // (the admin who drew gets data via HTTP response; this WS message
            // keeps other admin tabs in sync or arrives from a delayed broadcast)
            if (this.view === 'admin' && this.currentGame) {
                if (!this.currentGame.called_numbers) {
                    this.currentGame.called_numbers = [];
                }
                const alreadyHas = this.currentGame.called_numbers.includes(drawn.number);
                if (!alreadyHas) {
                    this.currentGame.called_numbers.push(drawn.number);
                    this.currentGame.total_called = this.currentGame.called_numbers.length;
                    this.lastDrawn = drawn;
                }

                const prevWinnerCount = this.winners.length;
                if (msg.winners) {
                    this.winners = msg.winners;
                }
                if (this.winners.length > prevWinnerCount && this.winners.length > 0) {
                    this.notify('We have winner(s)!', 'success');
                }
            }
        },

        /**
         * Handle a cards_update message pushed from the server.
         */
        _handleCardsUpdate(msg) {
            if (this.view === 'admin') {
                this.cards = msg.cards || [];
            }
        },

        /**
         * Handle a patterns_update message pushed from the server.
         */
        _handlePatternsUpdate(msg) {
            if (this.view === 'admin') {
                this.patterns = msg.patterns || [];
                if (msg.categories) this.categories = msg.categories;
            }
        },

        /**
         * Handle a card_deleted message. The server sends this when the player's
         * card has been deleted. Log the player out and return to the home screen.
         */
        _handleCardDeleted() {
            if (this.view === 'player') {
                // Prevent reconnect loop — the card no longer exists
                this.disconnectWS();
                this.playerCard = null;
                this.playerGame = null;
                this.stamps = {};
                this.view = 'home';
                this.notify('Your card has been deleted. You have been logged out.', 'error');
            }
        },

        /* ====== Player ====== */

        /** Joins a bingo game by card ID. Fetches the board and game state, then opens WebSocket. */
        async joinGame() {
            this.joinError = '';
            if (!this.joinId.trim()) return;
            try {
                const data = await this.api('board?id=' + encodeURIComponent(this.joinId.trim()));
                this.playerCard = data.card;
                this.playerGame = data.game;
                this.gameDetails = data.game_details || '';
                this.view = 'player';
                this.loadStamps();
                this.connectWS();
            } catch (e) {
                this.joinError = e.message;
            }
        },

        /** Disconnects WebSocket and returns the player to the home screen. */
        leaveGame() {
            this.disconnectWS();
            this.playerCard = null;
            this.playerGame = null;
            this.stamps = {};
            this.view = 'home';
        },

        /* ====== Admin Auth ====== */

        /** Navigates to the admin login screen. */
        goAdminLogin() {
            this.authError = '';
            this.adminPassword = '';
            this.view = 'admin-login';
        },

        /** Authenticates with the admin password, loads initial data, and opens WebSocket. */
        async adminLogin() {
            this.authError = '';
            try {
                const pw = this.adminPassword;
                this.adminPassword = '';
                await this.api('auth', {method: 'POST', body: {action: 'login', password: pw}});
            } catch (e) {
                this.authError = e.message;
                return;
            }
            // Auth succeeded -- load data, then show admin page
            try {
                await Promise.all([this.loadCards(), this.loadPatterns(), this.loadGameState(), this.loadRaffles(), this.loadSettings()]);
                this.drawDelay = parseInt(this.appSettings.default_draw_delay) || 0;
            } catch (e) {
                // Data loading had an unexpected error -- still proceed to admin
            }
            this.view = 'admin';
            this.connectWS();
        },

        /** Logs the admin out, disconnects WebSocket, and returns to home. */
        async adminLogout() {
            this.disconnectWS();
            try {
                await this.api('auth', {method: 'POST', body: {action: 'logout'}});
            } catch (e) {
            }
            this.view = 'home';
        },

        /* ====== Admin Nav ====== */

        /** Navigates to an admin tab, loading relevant data as needed. */
        adminNav(tab) {
            // Determine section from tab prefix
            if (tab.startsWith('bingo-')) this.adminSection = 'bingo';
            else if (tab.startsWith('raffle-')) this.adminSection = 'raffles';
            else if (tab.startsWith('system-')) this.adminSection = 'system';
            this.adminTab = tab;
            // Load data as needed
            if (tab === 'raffle-open' || tab === 'raffle-closed') {
                this.selectedRaffle = null;
                this.loadRaffles();
            }
            if (tab === 'system-themes') this.loadStyles();
            if (tab === 'system-settings') this.loadSettings();
            if (tab === 'bingo-winners-log') this.loadWinnersLog();
            if (tab === 'raffle-new' && !this.raffleForm) this.newRaffleForm();
        },
        toggleSection(section) {
            if (this.adminSection === section) return; // already open
            this.adminSection = section;
            // Navigate to default page for section
            if (section === 'bingo') this.adminNav('bingo-game');
            else if (section === 'raffles') this.adminNav('raffle-open');
            else if (section === 'system') this.adminNav('system-settings');
        },

        /* ====== Cards ====== */

        /** Loads all card IDs (with player names) from the server for the admin cards list. */
        async loadCards() {
            try {
                const data = await this.api('cards');
                this.cards = data.cards;
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Generates new bingo cards (1–500) and adds them to the cards list. */
        async generateCards() {
            try {
                const data = await this.api('cards', {
                    method: 'POST', body: {action: 'generate', count: this.generateCount}
                });
                this.notify(`Generated ${data.count} card(s)`, 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteCard(id) {
            try {
                await this.api('cards', {method: 'POST', body: {action: 'delete', id}});
                this.cards = this.cards.filter(c => c.id !== id);
                this.notify('Card deleted', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteAllCards() {
            if (!confirm('Delete ALL cards? This cannot be undone.')) return;
            try {
                await this.api('cards', {method: 'POST', body: {action: 'delete_all'}});
                this.cards = [];
                this.notify('All cards deleted', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /**
         * Fetch a single card's board data on demand for the preview modal.
         */
        async openCardPreview(id) {
            if (this.previewLoading) return;
            this.previewLoading = true;
            try {
                const data = await this.api('board?id=' + encodeURIComponent(id) + '&preview=1');
                this.previewCard = data.card;
                this.previewCardEditing = null;
            } catch (e) {
                this.notify(e.message, 'error');
            } finally {
                this.previewLoading = false;
            }
        },

        /* ====== Patterns ====== */

        /** Loads all patterns and categories from the server. */
        async loadPatterns() {
            try {
                const data = await this.api('patterns');
                this.patterns = data.patterns;
                this.categories = data.categories || [];
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Resets the pattern editor (name and grid) to defaults. */
        clearPatternEditor() {
            this.newPatternName = '';
            this.newPatternGrid = this.emptyGrid();
        },

        togglePatternsCollapsed() {
            this.patternsCollapsed = !this.patternsCollapsed;
            // Set all categories to the new collapsed state
            const newState = {};
            for (const cat of this.categories) {
                newState[cat.id] = this.patternsCollapsed;
            }
            this.collapsedCategories = newState;
        },

        toggleCategoryCollapsed(catId) {
            this.collapsedCategories = {
                ...this.collapsedCategories,
                [catId]: !this.collapsedCategories[catId]
            };
        },

        isCategoryCollapsed(catId) {
            return !!this.collapsedCategories[catId];
        },

        /** Validates and saves a new pattern. Checks for duplicates client-side and server-side. */
        async savePattern() {
            const name = this.newPatternName.trim();
            if (!name) return;
            // Ensure at least one cell is selected
            const hasCell = this.newPatternGrid.some(r => r.some(c => c));
            if (!hasCell) {
                this.notify('Select at least one cell in the pattern', 'error');
                return;
            }
            // Client-side duplicate check
            const dup = this.patterns.find(p => {
                if (!p.pattern_data || p.pattern_data.length !== 5) return false;
                for (let r = 0; r < 5; r++) {
                    for (let c = 0; c < 5; c++) {
                        if (!!p.pattern_data[r][c] !== !!this.newPatternGrid[r][c]) return false;
                    }
                }
                return true;
            });
            if (dup) {
                const catName = dup.category_name || 'Unknown';
                this.notify(`Duplicate pattern! Matches "${dup.name}" in category "${catName}"`, 'error');
                return;
            }
            try {
                const catId = this.newPatternCategoryId || (this.categories.length ? this.categories[0].id : 1);
                await this.api('patterns', {
                    method: 'POST', body: {action: 'create', name, pattern_data: this.newPatternGrid, category_id: catId}
                });
                this.notify('Pattern saved', 'success');
                this.clearPatternEditor();
                await this.loadPatterns();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deletePattern(id) {
            try {
                await this.api('patterns', {method: 'POST', body: {action: 'delete', id}});
                this.patterns = this.patterns.filter(p => p.id !== id);
                this.notify('Pattern deleted', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async renamePattern(id) {
            const pat = this.patterns.find(p => p.id === id);
            if (!pat) return;
            const newName = prompt('Rename pattern:', pat.name);
            if (newName === null || newName.trim() === '' || newName.trim() === pat.name) return;
            try {
                await this.api('patterns', {
                    method: 'POST', body: {action: 'rename', id, name: newName.trim()}
                });
                pat.name = newName.trim();
                this.notify('Pattern renamed', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /**
         * Moves a pattern up or down within its category. Uses optimistic local swap
         * with server-side persistence; reverts on failure.
         */
        async movePattern(id, direction) {
            // Find the pattern's category and do optimistic local reorder within it.
            const pat = this.patterns.find(p => p.id === id);
            if (!pat) return;
            const catPatterns = this.patterns.filter(p => p.category_id === pat.category_id);
            const idx = catPatterns.findIndex(p => p.id === id);
            const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
            if (swapIdx < 0 || swapIdx >= catPatterns.length) return;

            // Swap in the main array
            const mainIdx = this.patterns.indexOf(catPatterns[idx]);
            const mainSwapIdx = this.patterns.indexOf(catPatterns[swapIdx]);
            const copy = [...this.patterns];
            [copy[mainIdx], copy[mainSwapIdx]] = [copy[mainSwapIdx], copy[mainIdx]];
            this.patterns = copy;

            // Persist to server in background
            try {
                await this.api('patterns', {
                    method: 'POST', body: {action: 'reorder', id, direction}
                });
            } catch (e) {
                this.notify(e.message, 'error');
                await this.loadPatterns(); // revert to server state on error
            }
        },

        async setPatternCategory(patternId, categoryId) {
            try {
                await this.api('patterns', {
                    method: 'POST', body: {action: 'set_category', id: patternId, category_id: categoryId}
                });
                await this.loadPatterns();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /* ====== Pattern Categories ====== */
        async createCategory() {
            const name = (this.newCategoryName || '').trim();
            if (!name) return;
            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'create', name}
                });
                this.newCategoryName = '';
                this.notify('Category created', 'success');
                await this.loadPatterns();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async renameCategory(id) {
            const cat = this.categories.find(c => c.id === id);
            if (!cat) return;
            const newName = prompt('Rename category:', cat.name);
            if (newName === null || newName.trim() === '' || newName.trim() === cat.name) return;
            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'rename', id, name: newName.trim()}
                });
                cat.name = newName.trim();
                this.notify('Category renamed', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteCategory(id) {
            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'delete', id}
                });
                this.notify('Category deleted', 'info');
                await this.loadPatterns();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async moveCategoryOrder(id, direction) {
            // Optimistic local swap
            const idx = this.categories.findIndex(c => c.id === id);
            if (idx === -1) return;
            const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
            if (swapIdx < 0 || swapIdx >= this.categories.length) return;
            const copy = [...this.categories];
            [copy[idx], copy[swapIdx]] = [copy[swapIdx], copy[idx]];
            this.categories = copy;

            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'reorder', id, direction}
                });
            } catch (e) {
                this.notify(e.message, 'error');
                await this.loadPatterns();
            }
        },

        /* ====== Inline Editing: Categories ====== */
        startCategoryRename(cat) {
            this.editingCategoryId = cat.id;
            this.editingCategoryName = cat.name;
            this.$nextTick(() => {
                const input = this.$el.querySelector('.category-chip .inline-edit-input');
                if (input) input.focus();
            });
        },

        async finishCategoryRename(id) {
            const newName = (this.editingCategoryName || '').trim();
            this.editingCategoryId = null;
            if (!newName) return;
            const cat = this.categories.find(c => c.id === id);
            if (!cat || cat.name === newName) return;
            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'rename', id, name: newName}
                });
                cat.name = newName;
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        confirmDeleteCategory(id) {
            if (!confirm('Delete this category? Its patterns will be moved to the first remaining category.')) return;
            this.deleteCategory(id);
        },

        /* ====== Inline Editing: Patterns ====== */
        startPatternRename(pat) {
            this.editingPatternId = pat.id;
            this.editingPatternName = pat.name;
            this.$nextTick(() => {
                const input = this.$el.querySelector('.saved-pattern .pattern-name-input');
                if (input) input.focus();
            });
        },

        async finishPatternRename(id) {
            const newName = (this.editingPatternName || '').trim();
            this.editingPatternId = null;
            if (!newName) return;
            const pat = this.patterns.find(p => p.id === id);
            if (!pat || pat.name === newName) return;
            try {
                await this.api('patterns', {
                    method: 'POST', body: {action: 'rename', id, name: newName}
                });
                pat.name = newName;
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        confirmDeletePattern(id) {
            if (!confirm('Delete this pattern?')) return;
            this.deletePattern(id);
        },

        /* ====== Game ====== */

        /** Fetches the current game state (called numbers, patterns, winners) from the server. */
        async loadGameState() {
            try {
                const data = await this.api('game');
                this.currentGame = data.game;
                this.winners = data.winners || [];
                this.gameDetails = data.game_details || '';
                this.loadFrequentWinners();
            } catch (e) { /* silent */
            }
        },

        /** Starts a new game with the selected pattern IDs. Ends any active game first. */
        async startGame() {
            if (this.selectedPatternIds.length === 0) {
                this.notify('Select at least one win pattern', 'error');
                return;
            }
            try {
                const data = await this.api('game', {
                    method: 'POST', body: {action: 'start', pattern_ids: this.selectedPatternIds}
                });
                this.currentGame = data.game;
                this.winners = [];
                this.lastDrawn = null;
                this.selectedPatternIds = [];
                if (data.game_details !== undefined) this.gameDetails = data.game_details;
                this.notify('Game started!', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /**
         * Draws a random number from the remaining pool. Updates game state,
         * manages the player broadcast delay countdown, and triggers halftime
         * prompt at 35 numbers drawn.
         */
        async drawNumber() {
            try {
                const delay = this.drawDelay || 0;
                const prevCount = this.winners.length;
                const data = await this.api('game', {method: 'POST', body: {action: 'draw', delay}});
                this.lastDrawn = data.drawn;
                this.currentGame = data.game;
                this.winners = data.winners || [];
                if (this.winners.length > prevCount) {
                    this.notify('We have winner(s)!', 'success');
                }

                // Start countdown if delay > 0, otherwise show instant "sent"
                this._clearDrawCountdown();
                if (delay > 0) {
                    this.drawSent = false;
                    this.drawCountdown = delay;
                    this._drawCountdownTimer = setInterval(() => {
                        this.drawCountdown--;
                        if (this.drawCountdown <= 0) {
                          this._clearDrawCountdown();
                          this.drawSent = true;
                          setTimeout(() => { this.drawSent = false; }, 3000);
                        }
                    }, 1000);
                } else {
                    this.drawCountdown = null;
                    this.drawSent = false;
                }

                // After the 35th number is drawn, prompt admin about halftime minigame
                if (this.currentGame && this.currentGame.called_numbers && this.currentGame.called_numbers.length === 35) {
                    this.showHalftimePrompt = true;
                }
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Clears the draw countdown interval timer and resets state. */
        _clearDrawCountdown() {
            if (this._drawCountdownTimer) {
                clearInterval(this._drawCountdownTimer);
                this._drawCountdownTimer = null;
            }
            this.drawCountdown = null;
        },

        /**
         * Ends the current game. If there are winners, shows a confirmation modal
         * to let the admin select valid winners before logging them.
         */
        async endGame() {
            if (this.winners.length > 0) {
                // Show modal to select valid winners
                this.endGameSelectedWinners = [...this.winners];
                this.showEndGameModal = true;
                return;
            }
            await this.confirmEndGame([]);
        },

        /** Confirms end of game with the selected valid winner card IDs to log. */
        async confirmEndGame(validWinnerIds) {
            this.showEndGameModal = false;
            try {
                await this.api('game', {method: 'POST', body: {action: 'end', valid_winner_ids: validWinnerIds}});
                this.currentGame = null;
                this.winners = [];
                this.lastDrawn = null;
                this.winnerPreview = null;
                this._clearDrawCountdown();
                this.drawSent = false;
                this.notify('Game ended', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Persists the game details text to the server and broadcasts to all clients. */
        async saveGameDetails() {
            try {
                await this.api('game', {
                    method: 'POST', body: {action: 'update_details', details: this.gameDetails}
                });
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        // ── Winners Log ─────────────────────────────────────────────────────

        /** Fetches paginated winners log entries from the server. */
        async loadWinnersLog() {
            try {
                const data = await this.api(`winners-log?page=${this.winnersLogPage}&per_page=${this.winnersLogPerPage}&sort=${this.winnersLogSort}&dir=${this.winnersLogDir}`);
                this.winnersLog = data.entries || [];
                this.winnersLogTotal = data.total || 0;
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Computes total pages for winners log pagination. */
        winnersLogTotalPages() {
            return Math.ceil(this.winnersLogTotal / this.winnersLogPerPage) || 1;
        },

        /** Toggles sort direction or switches sort field, then reloads the log. */
        winnersLogSetSort(field) {
            if (this.winnersLogSort === field) {
                this.winnersLogDir = this.winnersLogDir === 'asc' ? 'desc' : 'asc';
            } else {
                this.winnersLogSort = field;
                this.winnersLogDir = 'desc';
            }
            this.winnersLogPage = 1;
            this.loadWinnersLog();
        },

        winnersLogGoPage(p) {
            this.winnersLogPage = p;
            this.loadWinnersLog();
        },

        /** Fetches players who have won 3+ times in the last 12 hours. */
        async loadFrequentWinners() {
            try {
                const data = await this.api('winners-log/frequent');
                this.frequentWinners = data.winners || [];
            } catch (e) { /* silent */ }
        },

        // ── Card Player Editing ─────────────────────────────────────────────

        /** Starts inline editing of a field on the preview card. */
        startPreviewCardEdit(field) {
            this.previewCardEditing = field;
            this.previewCardEditValue = this.previewCard[field] || '';
            this.$nextTick(() => {
                const input = this.$refs.previewEditInput;
                if (input) input.focus();
            });
        },

        /** Saves the inline-edited field on the preview card to the server. */
        async savePreviewCardField(field) {
            const newValue = this.previewCardEditValue.trim();
            const oldValue = this.previewCard[field] || '';
            this.previewCardEditing = null;
            if (newValue === oldValue) return;
            try {
                const payload = {action: 'update_player', id: this.previewCard.id};
                payload.player_name = field === 'player_name' ? newValue : (this.previewCard.player_name || '');
                payload.details = field === 'details' ? newValue : (this.previewCard.details || '');
                await this.api('cards', {method: 'POST', body: payload});
                this.previewCard[field] = newValue;
                const card = this.cards.find(c => c.id === this.previewCard.id);
                if (card) {
                    card.player_name = payload.player_name;
                    card.details = payload.details;
                }
                this.notify('Card updated', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /**
         * Fetches a winning card's board and highlights cells that match the
         * active win patterns, showing which cells constitute the winning combination.
         */
        async viewWinner(cardId) {
            if (this.winnerLoading) return;
            this.winnerLoading = true;
            try {
                const data = await this.api('board?id=' + encodeURIComponent(cardId));
                const card = data.card;
                const calledSet = this.adminCalledSet;
                const patterns = this.currentGame ? this.currentGame.patterns : [];
                // Find cells that complete any winning pattern
                const matchedCells = new Set();
                for (const pat of patterns) {
                    const pd = pat.pattern_data;
                    let satisfied = true;
                    for (let r = 0; r < 5; r++) {
                        for (let c = 0; c < 5; c++) {
                            if (pd[r][c]) {
                                const val = card.board_data[r][c];
                                if (val !== 0 && !calledSet.has(val)) {
                                    satisfied = false;
                                    break;
                                }
                            }
                        }
                        if (!satisfied) break;
                    }
                    if (satisfied) {
                        for (let r = 0; r < 5; r++) {
                            for (let c = 0; c < 5; c++) {
                                if (pd[r][c]) matchedCells.add(r + '-' + c);
                            }
                        }
                    }
                }
                this.winnerPreview = {card, matchedCells};
            } catch (e) {
                this.notify(e.message, 'error');
            } finally {
                this.winnerLoading = false;
            }
        },

        isWinnerCellMatch(ri, ci) {
            if (!this.winnerPreview) return false;
            return this.winnerPreview.matchedCells.has(ri + '-' + ci);
        },

        /* ====== Halftime Minigame ====== */
        async confirmHalftime() {
            this.showHalftimePrompt = false;
            try {
                await this.api('game', {method: 'POST', body: {action: 'trigger_halftime'}});
                this.notify('Halftime alert sent to all players!', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        dismissHalftime() {
            this.showHalftimePrompt = false;
        },

        dismissMinigameModal() {
            this.showMinigameModal = false;
        },

        /* ====== App Settings ====== */

        /** Loads app settings from the server and applies them to the UI. */
        async loadSettings() {
            try {
                const data = await this.api('settings');
                if (data.settings) {
                    this.appSettings = {...this.appSettings, ...data.settings};
                    document.title = this.appSettings.app_title || 'Senpan App Suite';
                    this._applyHeaderFont(this.appSettings.header_font);
                    this.loadGoogleFontsList();
                }
            } catch (e) { /* silent */ }
        },

        /** Saves the current app settings to the server. */
        async saveSettings() {
            try {
                await this.api('settings', {
                    method: 'POST',
                    body: {settings: this.appSettings}
                });
                document.title = this.appSettings.app_title || 'Senpan App Suite';
                this._applyHeaderFont(this.appSettings.header_font);
                this.loadGoogleFontsList();
                this.notify('Settings saved!', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Fetches the list of Google Fonts from the API if an API key is configured. Caches the result. */
        async loadGoogleFontsList() {
            const key = (this.appSettings.google_fonts_api_key || '').trim();
            if (!key) { this.googleFontsList = []; this._googleFontsCacheKey = ''; return; }
            // Skip fetch if the key hasn't changed since last successful load.
            if (key === this._googleFontsCacheKey && this.googleFontsList.length > 0) return;
            try {
                const resp = await fetch(`https://www.googleapis.com/webfonts/v1/webfonts?key=${encodeURIComponent(key)}&sort=popularity`);
                if (!resp.ok) { this.googleFontsList = []; return; }
                const data = await resp.json();
                this.googleFontsList = (data.items || []).map(f => f.family);
                this._googleFontsCacheKey = key;
            } catch { this.googleFontsList = []; }
        },

        /**
         * Loads a Google Font by family name and sets the --header-font CSS variable.
         * Injects a <link> tag for the Google Fonts CSS if not already present.
         * @param {string} fontFamily - Google Font family name (e.g. "Arapey", "Playfair Display")
         */
        _applyHeaderFont(fontFamily) {
            if (!fontFamily) fontFamily = 'Arapey';
            // Set CSS custom property
            document.documentElement.style.setProperty('--header-font', `'${fontFamily}', serif`);
            // Load from Google Fonts if not already loaded
            this._loadGoogleFont(fontFamily);
        },

        /**
         * Injects a Google Fonts <link> stylesheet for the given font family.
         * Uses a data attribute to track which fonts have been loaded to avoid duplicates.
         * @param {string} fontFamily - Google Font family name
         */
        _loadGoogleFont(fontFamily) {
            if (!fontFamily) return;
            const id = 'gfont-' + fontFamily.replace(/\s+/g, '-').toLowerCase();
            if (document.getElementById(id)) return; // already loaded
            const link = document.createElement('link');
            link.id = id;
            link.rel = 'stylesheet';
            link.href = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(fontFamily)}:ital,wght@0,400;0,700;0,800;1,400&display=swap`;
            document.head.appendChild(link);
        },

        /**
         * Previews a Google Font in the settings panel without saving.
         * Loads the font and applies it to the CSS variable so the preview text updates live.
         */
        previewHeaderFont() {
            const font = (this.appSettings.header_font || '').trim();
            if (font) {
                this._applyHeaderFont(font);
            }
        },

        /* ====== Styles ====== */

        /** Loads the list of all custom CSS themes and the active theme ID. */
        async loadStyles() {
            this._destroyCodeMirror();
            this.editingStyle = null;
            try {
                const data = await this.api('styles');
                this.styles = data.styles || [];
                this.activeStyleId = data.active_style_id || '';
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async loadStyle(id) {
            try {
                const data = await this.api('styles', {
                    method: 'POST', body: {action: 'get', id}
                });
                this.editingStyle = data.style;
                this.$nextTick(() => this._initCodeMirror());
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        newStyle() {
            this.editingStyle = {id: 0, name: '', css_content: ''};
            this.$nextTick(() => this._initCodeMirror());
        },

        async saveStyle() {
            if (!this.editingStyle) return;
            // Flush any pending CodeMirror debounce
            if (this._cmEditor) {
                clearTimeout(this._cmDebounce);
                this.editingStyle.css_content = this._cmEditor.getValue();
            }
            const name = this.editingStyle.name.trim();
            if (!name) {
                this.notify('Theme name is required', 'error');
                return;
            }
            try {
                if (this.editingStyle.id) {
                    await this.api('styles', {
                        method: 'POST',
                        body: {
                            action: 'update',
                            id: this.editingStyle.id,
                            name,
                            css_content: this.editingStyle.css_content
                        }
                    });
                    this.notify('Theme saved', 'success');
                } else {
                    const data = await this.api('styles', {
                        method: 'POST',
                        body: {action: 'create', name, css_content: this.editingStyle.css_content}
                    });
                    this.editingStyle.id = data.id;
                    this.notify('Theme created', 'success');
                }
                await this.loadStyles();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteStyle(id) {
            if (!confirm('Delete this theme?')) return;
            try {
                await this.api('styles', {method: 'POST', body: {action: 'delete', id}});
                if (this.editingStyle && this.editingStyle.id === id) {
                    this._destroyCodeMirror();
                    this.editingStyle = null;
                }
                this.notify('Theme deleted', 'info');
                await this.loadStyles();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async setActiveStyle(id) {
            try {
                await this.api('styles', {method: 'POST', body: {action: 'set_active', id}});
                this.activeStyleId = id > 0 ? String(id) : '';
                // Apply the CSS locally immediately
                if (id > 0 && this.editingStyle && this.editingStyle.id === id) {
                    this._applyCustomCSS(this.editingStyle.css_content || '');
                } else if (id > 0) {
                    await this._loadActiveCSS();
                } else {
                    this._applyCustomCSS('');
                }
                this.notify(id > 0 ? 'Theme activated' : 'Theme cleared', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        /** Fetches the active theme's CSS on page load and injects it into the DOM. */
        async _loadActiveCSS() {
            try {
                const data = await this.api('styles/active');
                this._applyCustomCSS(data.css || '');
            } catch (e) {
                // Silent — custom CSS is optional
            }
        },

        /** Injects or updates the custom CSS <style> element in the document head. */
        _applyCustomCSS(css) {
            let el = document.getElementById('bingo-custom-theme');
            if (!el) {
                el = document.createElement('style');
                el.id = 'bingo-custom-theme';
                document.head.appendChild(el);
            }
            el.textContent = css;
        },

        /** Initializes the CodeMirror CSS editor instance in the style editor panel. */
        _initCodeMirror() {
            this._destroyCodeMirror();
            const container = document.getElementById('style-editor-cm');
            if (!container || typeof CodeMirror === 'undefined') return;

            this._cmEditor = CodeMirror(container, {
                value: this.editingStyle ? this.editingStyle.css_content : '',
                mode: 'css',
                theme: 'default',
                lineNumbers: true,
                lineWrapping: true,
                indentUnit: 4,
                tabSize: 4,
                indentWithTabs: false,
                matchBrackets: true,
                autoCloseBrackets: true,
            });

            this._cmEditor.on('change', (cm) => {
                clearTimeout(this._cmDebounce);
                this._cmDebounce = setTimeout(() => {
                    if (this.editingStyle) {
                        this.editingStyle.css_content = cm.getValue();
                    }
                }, 300);
            });
        },

        /** Destroys the CodeMirror instance and removes its DOM element. */
        _destroyCodeMirror() {
            clearTimeout(this._cmDebounce);
            if (this._cmEditor) {
                const wrapper = this._cmEditor.getWrapperElement();
                if (wrapper && wrapper.parentNode) {
                    wrapper.parentNode.removeChild(wrapper);
                }
                this._cmEditor = null;
            }
        },

        /* ====== Drag & Drop: Categories ====== */

        /** Initiates drag of a pattern category chip. Sets transfer data and placeholder state. */
        onCategoryDragStart(event, catId) {
            event.dataTransfer.effectAllowed = 'move';
            event.dataTransfer.setData('text/plain', JSON.stringify({type: 'category', id: catId}));
            event.target.classList.add('dragging');
            this.categoryDragPlaceholder = null;
        },
        onCategoryDragEnd(event) {
            event.target.classList.remove('dragging');
            this.categoryDragPlaceholder = null;
        },
        onCategoryDragOver(event, catId) {
            event.preventDefault();
            event.dataTransfer.dropEffect = 'move';
            if (!catId) return;
            const el = event.currentTarget;
            if (el.classList.contains('dragging')) return;
            const rect = el.getBoundingClientRect();
            const midX = rect.left + rect.width / 2;
            if (event.clientX < midX) {
                this.categoryDragPlaceholder = {beforeId: catId};
            } else {
                this.categoryDragPlaceholder = {afterId: catId};
            }
        },
        onCategoryDragEnter(event, catId) {},
        onCategoryDragLeave(event) {},
        onCategoryDropToGrid(event) {
            // Handle drops on placeholder/grid area using placeholder state
            const ph = this.categoryDragPlaceholder;
            if (!ph) return;
            const targetCatId = ph.beforeId || ph.afterId;
            if (targetCatId) this.onCategoryDrop(event, targetCatId);
        },
        async onCategoryDrop(event, targetCatId) {
            event.preventDefault();
            const placeholder = this.categoryDragPlaceholder;
            this.categoryDragPlaceholder = null;
            let data;
            try { data = JSON.parse(event.dataTransfer.getData('text/plain')); } catch { return; }
            if (data.type !== 'category' || data.id === targetCatId) return;

            const fromIdx = this.categories.findIndex(c => c.id === data.id);
            let toIdx = this.categories.findIndex(c => c.id === targetCatId);
            if (fromIdx === -1 || toIdx === -1) return;

            if (placeholder && placeholder.afterId === targetCatId) toIdx++;
            if (fromIdx < toIdx) toIdx--;

            const copy = [...this.categories];
            const [moved] = copy.splice(fromIdx, 1);
            copy.splice(toIdx, 0, moved);
            this.categories = copy;

            try {
                await this.api('pattern-categories', {
                    method: 'POST', body: {action: 'bulk_reorder', ordered_ids: copy.map(c => c.id)}
                });
            } catch (e) {
                this.notify(e.message, 'error');
                await this.loadPatterns();
            }
        },

        /* ====== Drag & Drop: Patterns ====== */
        onPatternDragStart(event, patternId, categoryId) {
            event.dataTransfer.effectAllowed = 'move';
            event.dataTransfer.setData('text/plain', JSON.stringify({type: 'pattern', id: patternId, categoryId}));
            event.target.classList.add('dragging');
            this.patternDragPlaceholder = null;
        },
        onPatternDragEnd(event) {
            event.target.classList.remove('dragging');
            this.patternDragPlaceholder = null;
            document.querySelectorAll('.pattern-drop-zone').forEach(el => el.classList.remove('drag-over'));
        },
        onPatternDragOver(event) {
            event.preventDefault();
            event.dataTransfer.dropEffect = 'move';
            const el = event.currentTarget;
            if (!el.classList.contains('saved-pattern') || el.classList.contains('dragging')) return;
            const rect = el.getBoundingClientRect();
            const midX = rect.left + rect.width / 2;
            // Find pattern id and category from DOM position
            const zone = el.closest('.pattern-drop-zone');
            if (!zone) return;
            const allZones = Array.from(document.querySelectorAll('.pattern-drop-zone'));
            const zoneIdx = allZones.indexOf(zone);
            const group = this.patternsByCategory[zoneIdx];
            if (!group) return;
            const items = Array.from(zone.querySelectorAll('.saved-pattern'));
            const itemIdx = items.indexOf(el);
            const pat = group.patterns[itemIdx];
            if (!pat) return;
            if (event.clientX < midX) {
                this.patternDragPlaceholder = {categoryId: group.category.id, beforeId: pat.id};
            } else {
                this.patternDragPlaceholder = {categoryId: group.category.id, afterId: pat.id};
            }
        },
        onPatternDragEnter(event) {
            const el = event.currentTarget;
            if (el.classList.contains('pattern-drop-zone')) {
                el.classList.add('drag-over');
            }
        },
        onPatternDragLeave(event) {
            event.currentTarget.classList.remove('drag-over');
        },
        async onPatternDrop(event, targetPatternId, targetCategoryId) {
            // Delegate to zone-level handler which handles both same-category and cross-category
            await this.onPatternDropToCategory(event, targetCategoryId);
        },
        async onPatternDropToCategory(event, targetCategoryId) {
            event.preventDefault();
            const placeholder = this.patternDragPlaceholder;
            this.patternDragPlaceholder = null;
            document.querySelectorAll('.pattern-drop-zone').forEach(el => el.classList.remove('drag-over'));
            let data;
            try { data = JSON.parse(event.dataTransfer.getData('text/plain')); } catch { return; }
            if (data.type !== 'pattern') return;

            const draggedId = data.id;
            const sourceCatId = data.categoryId;

            if (sourceCatId === targetCategoryId) {
                // Same-category reorder using placeholder position
                if (!placeholder || placeholder.categoryId !== targetCategoryId) return;
                const catPatterns = this.patterns.filter(p => p.category_id === targetCategoryId);
                const fromIdx = catPatterns.findIndex(p => p.id === draggedId);
                if (fromIdx === -1) return;

                const targetId = placeholder.beforeId || placeholder.afterId;
                let toIdx = catPatterns.findIndex(p => p.id === targetId);
                if (toIdx === -1) return;
                const insertAfter = !!placeholder.afterId;
                if (insertAfter) toIdx++;
                if (fromIdx < toIdx) toIdx--;
                if (fromIdx === toIdx) return;

                const [moved] = catPatterns.splice(fromIdx, 1);
                catPatterns.splice(toIdx, 0, moved);

                const newPatterns = this.patterns.filter(p => p.category_id !== targetCategoryId);
                const firstIdx = this.patterns.findIndex(p => p.category_id === targetCategoryId);
                newPatterns.splice(firstIdx >= 0 ? firstIdx : newPatterns.length, 0, ...catPatterns);
                this.patterns = newPatterns;

                try {
                    await this.api('patterns', {
                        method: 'POST', body: {action: 'bulk_reorder', category_id: targetCategoryId, ordered_ids: catPatterns.map(p => p.id)}
                    });
                } catch (e) {
                    this.notify(e.message, 'error');
                    await this.loadPatterns();
                }
                return;
            }

            // Cross-category move
            const movedPattern = this.patterns.find(p => p.id === draggedId);
            if (!movedPattern) return;

            const targetCat = this.categories.find(c => c.id === targetCategoryId);
            movedPattern.category_id = targetCategoryId;
            movedPattern.category_name = targetCat ? targetCat.name : '';

            // Determine insertion position from placeholder
            const targetPatterns = this.patterns.filter(p => p.category_id === targetCategoryId && p.id !== draggedId);
            let insertIdx = targetPatterns.length;
            if (placeholder && placeholder.categoryId === targetCategoryId) {
                const refId = placeholder.beforeId || placeholder.afterId;
                const refIdx = targetPatterns.findIndex(p => p.id === refId);
                if (refIdx >= 0) insertIdx = placeholder.beforeId ? refIdx : refIdx + 1;
            }
            targetPatterns.splice(insertIdx, 0, movedPattern);

            // Rebuild patterns array
            const sourcePatterns = this.patterns.filter(p => p.category_id === sourceCatId && p.id !== draggedId);
            this.patterns = this.patterns.filter(p => p.id !== draggedId && p.category_id !== targetCategoryId);
            let catInsertIdx = this.patterns.length;
            for (let i = 0; i < this.categories.length; i++) {
                if (this.categories[i].id === targetCategoryId) {
                    for (let j = i + 1; j < this.categories.length; j++) {
                        const idx = this.patterns.findIndex(p => p.category_id === this.categories[j].id);
                        if (idx >= 0) { catInsertIdx = idx; break; }
                    }
                    break;
                }
            }
            this.patterns.splice(catInsertIdx, 0, ...targetPatterns);

            try {
                await this.api('patterns', {
                    method: 'POST', body: {action: 'bulk_reorder', category_id: targetCategoryId, ordered_ids: targetPatterns.map(p => p.id)}
                });
                if (sourcePatterns.length) {
                    await this.api('patterns', {
                        method: 'POST', body: {action: 'bulk_reorder', category_id: sourceCatId, ordered_ids: sourcePatterns.map(p => p.id)}
                    });
                }
            } catch (e) {
                this.notify(e.message, 'error');
                await this.loadPatterns();
            }
        },

        /* ====== Raffles ====== */
        async loadRaffles() {
            try {
                const data = await this.api('raffles');
                this.raffles = data.raffles || [];
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async loadRaffleDetail(id) {
            try {
                const data = await this.api('raffles/' + id);
                this.selectedRaffle = data.raffle;
                this.raffleEntries = data.entries || [];
                this.raffleWinner = null;
                // Check if there's a pending winner
                if (data.raffle.winner_entry_id && this.raffleEntries.length) {
                    this.raffleWinner = this.raffleEntries.find(e => e.id === data.raffle.winner_entry_id) || null;
                }
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        viewRaffle(raffle) {
            this.selectedRaffle = raffle;
            this.raffleSignup = {characterName: '', world: '', numEntries: 1};
            this.raffleSignupResult = null;
            this.raffleEntries = [];
            this.raffleWinner = null;
            this.loadRaffleDetail(raffle.id);
        },

        viewPublicRaffle(raffle) {
            this.selectedRaffle = raffle;
            this.raffleSignup = {characterName: '', world: '', numEntries: 1};
            this.raffleSignupResult = null;
            this.raffleWinnerEntry = null;
            this.raffleTotalEntryCount = 0;
            this.view = 'raffle-detail';
            // Fetch detail to get winner entry and total entries
            this.api('raffles/' + raffle.id).then(data => {
                this.selectedRaffle = data.raffle;
                this.raffleTotalEntryCount = data.total_entries || 0;
                if (data.winner_entry) {
                    this.raffleWinnerEntry = data.winner_entry;
                }
            }).catch(() => {});
        },

        backToRaffles() {
            this.selectedRaffle = null;
            this.raffleSignupResult = null;
            if (this.view === 'raffle-detail') {
                this.view = 'raffles';
            }
        },

        newRaffleForm() {
            this.raffleForm = {
                id: 0, title: '', description: '', rules: '', max_entries: 1,
                signup_instructions: '', cost_per_entry: 0,
                available_from: '', available_to: '', prize_image: ''
            };
            if (this.view === 'admin') this.adminNav('raffle-new');
        },

        editRaffleForm(raffle) {
            this.raffleForm = {...raffle};
        },

        cancelRaffleForm() {
            this.raffleForm = null;
        },

        async saveRaffle() {
            if (!this.raffleForm) return;
            const f = this.raffleForm;
            if (!f.title.trim()) {
                this.notify('Title is required', 'error');
                return;
            }
            try {
                if (f.id) {
                    await this.api('raffles', {method: 'POST', body: {action: 'update', ...f}});
                    this.notify('Raffle updated', 'success');
                } else {
                    await this.api('raffles', {method: 'POST', body: {action: 'create', ...f}});
                    this.notify('Raffle created', 'success');
                }
                this.raffleForm = null;
                await this.loadRaffles();
                // Navigate to open raffles after save
                this.adminNav('raffle-open');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteRaffle(id) {
            if (!confirm('Delete this raffle and all its entries?')) return;
            try {
                await this.api('raffles', {method: 'POST', body: {action: 'delete', id}});
                this.raffles = this.raffles.filter(r => r.id !== id);
                if (this.selectedRaffle && this.selectedRaffle.id === id) {
                    this.selectedRaffle = null;
                }
                this.notify('Raffle deleted', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async enterRaffle() {
            if (!this.selectedRaffle) return;
            const s = this.raffleSignup;
            if (!s.characterName.trim() || !s.world.trim()) {
                this.notify('Character name and world are required', 'error');
                return;
            }
            try {
                const data = await this.api('raffles/' + this.selectedRaffle.id + '/enter', {
                    method: 'POST',
                    body: {character_name: s.characterName.trim(), world: s.world.trim(), num_entries: s.numEntries}
                });
                this.raffleSignupResult = data;
                this.notify(data.message, 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        raffleTotalCost() {
            if (!this.selectedRaffle) return 0;
            return (this.raffleSignup.numEntries || 1) * this.selectedRaffle.cost_per_entry;
        },

        async toggleEntryPaid(entry) {
            try {
                await this.api('raffles/' + this.selectedRaffle.id + '/entries', {
                    method: 'POST',
                    body: {action: 'mark_paid', entry_id: entry.id, paid: !entry.paid}
                });
                entry.paid = !entry.paid;
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async deleteEntry(entry) {
            if (!confirm('Delete this entry?')) return;
            try {
                await this.api('raffles/' + this.selectedRaffle.id + '/entries', {
                    method: 'POST',
                    body: {action: 'delete_entry', entry_id: entry.id}
                });
                this.raffleEntries = this.raffleEntries.filter(e => e.id !== entry.id);
                this.notify('Entry deleted', 'info');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async pickRaffleWinner() {
            if (!this.selectedRaffle) return;
            try {
                const data = await this.api('raffles/' + this.selectedRaffle.id + '/entries', {
                    method: 'POST', body: {action: 'pick_winner'}
                });
                this.raffleWinner = data.winner;
                this.selectedRaffle.winner_entry_id = data.winner.id;
                this.notify('Winner picked!', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async verifyRaffleWinner() {
            if (!this.selectedRaffle) return;
            try {
                await this.api('raffles/' + this.selectedRaffle.id + '/entries', {
                    method: 'POST', body: {action: 'verify_winner'}
                });
                this.selectedRaffle.status = 'closed';
                this.notify('Winner verified! Raffle closed.', 'success');
                await this.loadRaffles();
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async pickAnotherWinner() {
            if (!this.selectedRaffle) return;
            try {
                const data = await this.api('raffles/' + this.selectedRaffle.id + '/entries', {
                    method: 'POST', body: {action: 'pick_another'}
                });
                this.raffleWinner = data.winner;
                this.selectedRaffle.winner_entry_id = data.winner.id;
                this.notify('New winner picked!', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            }
        },

        async uploadRaffleImage(event) {
            const file = event.target.files[0];
            if (!file) return;
            this.raffleImageUploading = true;
            try {
                const formData = new FormData();
                formData.append('image', file);
                const data = await this.api('raffles/upload', {method: 'POST', body: formData});
                if (this.raffleForm) {
                    this.raffleForm.prize_image = data.path;
                }
                this.notify('Image uploaded', 'success');
            } catch (e) {
                this.notify(e.message, 'error');
            } finally {
                this.raffleImageUploading = false;
            }
        },

        /** Renders Markdown text to HTML, falling back to escaped HTML with line breaks. */
        renderedMarkdown(text) {
            if (!text) return '';
            if (typeof marked !== 'undefined' && marked.parse) {
                return marked.parse(text, {breaks: true});
            }
            return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br>');
        },
    },

    /**
     * Vue lifecycle: loads the active CSS theme and preloads open raffles
     * for home page card visibility on initial mount.
     */
    mounted() {
        this._loadActiveCSS();
        this.loadSettings();
        // Preload open raffles to determine home card visibility
        this.api('raffles').then(data => {
            this.homeRaffles = (data.raffles || []).filter(r => r.status === 'open');
        }).catch(() => {});
    },

    /** Vue lifecycle: cleanly disconnects WebSocket before the app is destroyed. */
    beforeUnmount() {
        this.disconnectWS();
    },
}).mount('#app');

