package apidoc

import "app-suite/internal/model"

// modelTypes is every struct exposed on the wire — domain types plus response
// envelopes. Each becomes a named entry in components/schemas (nested types are
// inlined by the reflector). Keep in sync with the model package; the CI test
// fails if this list falls behind (a documented response would reference a
// missing schema, or the committed openapi.yaml would go stale).
var modelTypes = map[string]any{
	// Domain
	"Card": model.Card{}, "User": model.User{}, "TokenInfo": model.TokenInfo{},
	"PatternCategory": model.PatternCategory{}, "Pattern": model.Pattern{},
	"BingoGamePattern": model.BingoGamePattern{}, "GamePreset": model.GamePreset{},
	"BingoGameState": model.BingoGameState{}, "BingoDrawnNumber": model.BingoDrawnNumber{},
	"Style": model.Style{}, "Raffle": model.Raffle{}, "RaffleEntry": model.RaffleEntry{},
	"Garapon": model.Garapon{}, "GaraponPrize": model.GaraponPrize{},
	"GaraponPlayer": model.GaraponPlayer{}, "GaraponDraw": model.GaraponDraw{},
	"ReadingList": model.ReadingList{}, "ReadingListItem": model.ReadingListItem{},
	"ReadingListSource": model.ReadingListSource{}, "WinnersLogEntry": model.WinnersLogEntry{},
	"FrequentWinner": model.FrequentWinner{}, "AnnouncementType": model.AnnouncementType{},
	"AnnouncementRole": model.AnnouncementRole{}, "Announcement": model.Announcement{},
	"AnnouncementButton": model.AnnouncementButton{}, "Affiliate": model.Affiliate{},
	"AffiliateHour": model.AffiliateHour{}, "Placement": model.Placement{},
	"StampRally": model.StampRally{}, "StampRallyStamp": model.StampRallyStamp{},
	"StampRallyPrize": model.StampRallyPrize{}, "StampRallyCard": model.StampRallyCard{},
	"StampRallyCollected": model.StampRallyCollected{}, "StampRallyLogEntry": model.StampRallyLogEntry{},

	// Shared response envelopes
	"OKResponse": model.OKResponse{}, "DeletedResponse": model.DeletedResponse{},
	"DeletedCountResponse": model.DeletedCountResponse{}, "StatusResponse": model.StatusResponse{},
	"NamedOKResponse": model.NamedOKResponse{}, "RenamedResponse": model.RenamedResponse{},
	"PausedResponse": model.PausedResponse{}, "SkippedUpload": model.SkippedUpload{},
	"SettingsResponse": model.SettingsResponse{},

	// Auth / users / account
	"AuthCheckResponse": model.AuthCheckResponse{}, "LoginResponse": model.LoginResponse{},
	"LogoutResponse": model.LogoutResponse{}, "RegisterResponse": model.RegisterResponse{},
	"UsersResponse": model.UsersResponse{}, "AccountTokenGenerateResponse": model.AccountTokenGenerateResponse{},
	"TokenRevokeResponse": model.TokenRevokeResponse{},

	// Bingo
	"CardListEntry": model.CardListEntry{}, "CardsListResponse": model.CardsListResponse{},
	"GeneratedCard": model.GeneratedCard{}, "GenerateCardsResponse": model.GenerateCardsResponse{},
	"GeneratedNamedCard": model.GeneratedNamedCard{}, "GenerateSingleCardResponse": model.GenerateSingleCardResponse{},
	"CardResponse": model.CardResponse{}, "BoardResponse": model.BoardResponse{},
	"GameStateResponse": model.GameStateResponse{}, "DrawResult": model.DrawResult{},
	"EndGameResponse": model.EndGameResponse{}, "PatternsResponse": model.PatternsResponse{},
	"CreatedPattern": model.CreatedPattern{}, "PatternCreateResponse": model.PatternCreateResponse{},
	"CategoriesResponse": model.CategoriesResponse{}, "CategoryCreateResponse": model.CategoryCreateResponse{},
	"PresetsResponse": model.PresetsResponse{}, "PresetCreateResponse": model.PresetCreateResponse{},
	"StylesResponse": model.StylesResponse{}, "StyleGetResponse": model.StyleGetResponse{},
	"StyleCreateResponse": model.StyleCreateResponse{}, "ActiveCSSResponse": model.ActiveCSSResponse{},

	// Raffles / garapons
	"RafflesResponse": model.RafflesResponse{}, "RaffleResponse": model.RaffleResponse{},
	"RaffleDetailResponse": model.RaffleDetailResponse{}, "RaffleEnterResponse": model.RaffleEnterResponse{},
	"RaffleEntryResponse": model.RaffleEntryResponse{}, "RaffleWinnerResponse": model.RaffleWinnerResponse{},
	"GaraponsResponse": model.GaraponsResponse{}, "GaraponResponse": model.GaraponResponse{},
	"GaraponDetailResponse": model.GaraponDetailResponse{}, "GaraponPlayerResponse": model.GaraponPlayerResponse{},
	"PublicGarapon": model.PublicGarapon{}, "GaraponPublicPlayer": model.GaraponPublicPlayer{},
	"GaraponPublicResponse": model.GaraponPublicResponse{}, "GaraponDrawResponse": model.GaraponDrawResponse{},

	// Affiliates / stamp rally
	"AffiliatesResponse": model.AffiliatesResponse{}, "AffiliateResponse": model.AffiliateResponse{},
	"StampRalliesResponse": model.StampRalliesResponse{}, "StampRallyResponse": model.StampRallyResponse{},
	"StampRallyDetailResponse": model.StampRallyDetailResponse{}, "StampRallyCardResponse": model.StampRallyCardResponse{},
	"StampRallyLogsResponse": model.StampRallyLogsResponse{}, "PublicStampRally": model.PublicStampRally{},
	"PublicStamp": model.PublicStamp{}, "PublicPrize": model.PublicPrize{},
	"PublicStampCard": model.PublicStampCard{}, "StampSubmitResponse": model.StampSubmitResponse{},

	// Book club / announcements
	"ReadingListsResponse": model.ReadingListsResponse{}, "ReadingListDetailResponse": model.ReadingListDetailResponse{},
	"ReadingListItemResponse": model.ReadingListItemResponse{}, "BookclubLookupResponse": model.BookclubLookupResponse{},
	"BookclubUploadResponse": model.BookclubUploadResponse{}, "PublishResponse": model.PublishResponse{},
	"AnnouncementTypesResponse": model.AnnouncementTypesResponse{}, "AnnouncementTypeResponse": model.AnnouncementTypeResponse{},
	"AnnouncementRolesResponse": model.AnnouncementRolesResponse{}, "AnnouncementRoleResponse": model.AnnouncementRoleResponse{},
	"AnnouncementsResponse": model.AnnouncementsResponse{}, "AnnouncementResponse": model.AnnouncementResponse{},

	// Winners / settings / files
	"WinnersLogResponse": model.WinnersLogResponse{}, "FrequentWinnersResponse": model.FrequentWinnersResponse{},
	"FontFile": model.FontFile{}, "FontsResponse": model.FontsResponse{}, "FontUploadResponse": model.FontUploadResponse{},
	"CarrdProject": model.CarrdProject{}, "CarrdProjectsResponse": model.CarrdProjectsResponse{},
	"CarrdProjectCreateResponse": model.CarrdProjectCreateResponse{}, "CarrdImage": model.CarrdImage{},
	"CarrdImagesResponse": model.CarrdImagesResponse{}, "CarrdUploadResponse": model.CarrdUploadResponse{},
	"ImageCategory": model.ImageCategory{}, "ImageCategoriesResponse": model.ImageCategoriesResponse{},
	"ImageCategoryActionResponse": model.ImageCategoryActionResponse{}, "ImageEntry": model.ImageEntry{},
	"ImagesResponse": model.ImagesResponse{}, "ImagesUploadResponse": model.ImagesUploadResponse{},
}
