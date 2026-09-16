package leetcode

// GraphQL documents. Field aliases keep the JSON shape stable even if LeetCode
// renames underlying fields.

const qProblemList = `
query problemsetQuestionList($categorySlug: String, $limit: Int, $skip: Int, $filters: QuestionListFilterInput) {
  problemsetQuestionList: questionList(
    categorySlug: $categorySlug
    limit: $limit
    skip: $skip
    filters: $filters
  ) {
    total: totalNum
    questions: data {
      frontendId: questionFrontendId
      title
      slug: titleSlug
      difficulty
      acRate
      paidOnly: isPaidOnly
      status
      topicTags { slug }
    }
  }
}`

const qProblemCount = `
query problemCount($categorySlug: String, $filters: QuestionListFilterInput) {
  problemsetQuestionList: questionList(
    categorySlug: $categorySlug
    limit: 1
    skip: 0
    filters: $filters
  ) {
    total: totalNum
  }
}`

const qQuestionData = `
query questionData($titleSlug: String!) {
  question(titleSlug: $titleSlug) {
    questionId
    frontendId: questionFrontendId
    title
    slug: titleSlug
    difficulty
    acRate
    paidOnly: isPaidOnly
    status
    content
    exampleTestcases
    sampleTestCase
    metaData
    hints
    similarQuestions
    topicTags { slug name }
    codeSnippets { langSlug code }
  }
}`

const qStudyPlanDetail = `
query studyPlanV2Detail($planSlug: String!) {
  studyPlanV2Detail(planSlug: $planSlug) {
    slug
    name
    planSubGroups {
      slug
      name
      questions {
        slug: titleSlug
        frontendId: questionFrontendId
        title
        difficulty
      }
    }
  }
}`
