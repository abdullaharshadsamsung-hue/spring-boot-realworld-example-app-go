package app

import "encoding/json"

// ArticleDataList is the RealWorld list envelope for articles: one page of
// ArticleData plus the total number of articles matching the query (not
// just the size of the page). It serializes to
// {"articles": [...], "articlesCount": N}.
//
// Treat values as immutable once constructed; NewArticleDataList stores
// the given slice as-is, without copying.
type ArticleDataList struct {
	// Articles is the current page of articles (the source's articleDatas).
	Articles []ArticleData
	// Count is the total number of matching articles (the source's count).
	Count int
}

// NewArticleDataList returns a list wrapping articles and the total count.
// The slice is stored by reference, and the count is kept as given; neither
// is validated nor derived from the other.
func NewArticleDataList(articles []ArticleData, count int) ArticleDataList {
	return ArticleDataList{Articles: articles, Count: count}
}

// articleDataListJSON is the wire shape of ArticleDataList.
type articleDataListJSON struct {
	Articles      []ArticleData `json:"articles"`
	ArticlesCount int           `json:"articlesCount"`
}

// MarshalJSON serializes the list as {"articles": [...], "articlesCount": N}.
// A nil page is written as an empty array rather than null, so clients
// always receive an array.
func (l ArticleDataList) MarshalJSON() ([]byte, error) {
	articles := l.Articles
	if articles == nil {
		articles = []ArticleData{}
	}
	return json.Marshal(articleDataListJSON{
		Articles:      articles,
		ArticlesCount: l.Count,
	})
}
