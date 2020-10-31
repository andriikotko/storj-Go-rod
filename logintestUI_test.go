package storj

import (
	"fmt"
	"github.com/bmizerany/assert"
	"testing"
	"strings"


	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"

)
	var login string = "test1@g.com"
	var password = "123qwe"
	var startPage = "http://127.0.0.1:10002/login"
	var screenWidth int= 1350
	var screenHeigth int = 600

func Example_login() {

	page, browser := login_to_account()
	//check title
	fmt.Println(strings.Contains(page.MustElement(".dashboard-area__title").MustText(),"Dashboard"))
	// Output: true
	defer browser.MustClose()
}

func Example_logout()  {

	l := launcher.New().
		Headless(false).
		Devtools(false)
	defer l.Cleanup()
	url := l.MustLaunch()

	browser := rod.New().
		Timeout(time.Minute).
		ControlURL(url).
		Trace(true).
		Slowmotion(300 * time.Millisecond).
		MustConnect()

	// Even you forget to close, rod will close it after main process ends.
	defer browser.MustClose()

	// Timeout will be passed to all chained function calls.
	// The code will panic out if any chained call is used after the timeout.
	page := browser.Timeout(15*time.Second).MustPage(startPage)

	// Make sure viewport is always consistent.
	page.MustSetViewport(screenWidth, screenHeigth, 1, false)

	// We use css selector to get the search input element and input "git"
	page.MustElement(".headerless-input").MustInput(login)
	page.MustElement("[type=password]").MustInput(password)
	page.Keyboard.MustPress(input.Enter)
	// We use css selector to get the search
	page.MustElement(".account-button__container__avatar").MustClick()
	page.MustElement(".account-dropdown__wrap__item-container").MustClick()

	//check title
	fmt.Println(page.MustElement("h1.login-area__title-container__title").MustText())
	// Output: Login to Storj
}

func Example_SideMenuLinksChecking()  {
	page, browser := login_to_account()
	defer browser.MustClose()
	firstLink := *(page.MustElement("a.navigation-area__item-container:nth-of-type(1)",).MustAttribute("href"))
	fmt.Println(firstLink)
	// Output: /project-dashboard
	secondLink := *(page.MustElement("a.navigation-area__item-container:nth-of-type(2)",).MustAttribute("href"))
	fmt.Println(secondLink)
	// Output: /api-keys
	thirdLink := *(page.MustElement("a.navigation-area__item-container:nth-of-type(3)",).MustAttribute("href"))
	fmt.Println(thirdLink)
	// Output: /project-dashboard
	// /api-keys
	// /project-members

}
func login_to_account() (*rod.Page, *rod.Browser) {
	l := launcher.New().
		Headless(false).
		Devtools(false)
//	defer l.Cleanup()
	url := l.MustLaunch()

	browser := rod.New().
		Timeout(time.Minute).
		ControlURL(url).
		Trace(true).
		Slowmotion(300 * time.Millisecond).
		MustConnect()


	//// Even you forget to close, rod will close it after main process ends.
	//  defer browser.MustClose()

	// Timeout will be passed to all chained function calls.
	// The code will panic out if any chained call is used after the timeout.
	page := browser.Timeout(25*time.Second).MustPage(startPage)

	// Make sure viewport is always consistent.
	page.MustSetViewport(screenWidth, screenHeigth, 1, false)

	// We use css selector to get the search input element and input "git"
	page.MustElement(".headerless-input").MustInput(login)
	page.MustElement("[type=password]").MustInput(password)

	page.Keyboard.MustPress(input.Enter)

	return page, browser
}

func Example_droplistChosing (){
	page, browser := login_to_account()
	defer browser.MustClose()
	firstElement:= page.MustElement("div.resources-selection__toggle-container").MustClick().MustElement("a.resources-dropdown__item-container").MustText()
	fmt.Println(firstElement, page.MustInfo().URL)
	// Output: Docs http://127.0.0.1:10002/project-dashboard
}

	func Example_checkingElementsSideMenu(){
		page, browser := login_to_account()
		defer browser.MustClose()
		first := page.MustElement("a.navigation-area__item-container:nth-of-type(1)").MustText()
		fmt.Println(first)
		second := page.MustElement("a.navigation-area__item-container:nth-of-type(2)").MustText()
		fmt.Println(second)
		third := page.MustElement("a.navigation-area__item-container:nth-of-type(3)").MustText()
		fmt.Println(third)
		currentProject := page.MustHas("#app > div > div > div.dashboard__wrap__main-area > div.navigation-area.regular-navigation > div > div")
		fmt.Println(currentProject)
		// Output: Dashboard
		// API Keys
		// Users
		// true
	}

	func Example_checkingElementsHeader(){
		page, browser := login_to_account()
		browser.Slowmotion(1 * time.Second)
		defer browser.MustClose()

		projects := page.MustElement("div.project-selection__toggle-container").MustText()
		fmt.Println(projects)
		resources := page.MustElement("div.resources-selection__toggle-container").MustText()
		fmt.Println(resources)
		settings := page.MustElement("div.settings-selection__toggle-container").MustText()
		fmt.Println(settings)
		user := page.MustHas("div.account-button__container__avatar")
		fmt.Println(user)
		logo := page.MustHas("div.header-container__left-area__logo-area")
		fmt.Println(logo)
		// Output:
		// Projects
		// Resources
		// Settings
		// true
		// true
	}

	func Example_sideMenuEditProjectDroplist (){
		page, browser := login_to_account()
		defer browser.MustClose()
		text:= page.MustElement("div.edit-project").MustClick().MustElement("div.edit-project__dropdown").MustText()
		fmt.Println(text)
		// Output: Edit Details
	}

	func Example_editProjectScreen () {
		page, browser := login_to_account()
		defer browser.MustClose()
		currentProjectNameFromSideMenu := page.MustElement("div.edit-project").MustText()
		page.MustElement("div.edit-project").MustClick().MustElement("div.edit-project__dropdown").MustClick()
		projectDetailsHeader := page.MustElement("h1.project-details__wrapper__container__title").MustText()
		fmt.Println(projectDetailsHeader)
		projectNameHeader := page.MustElement("p.project-details__wrapper__container__label:nth-of-type(1)").MustText()
		fmt.Println(projectNameHeader)
		descriptionHeader := page.MustElement("p.project-details__wrapper__container__label:nth-of-type(2)").MustText()
		fmt.Println(descriptionHeader)
		projectNameFromEditScreen := page.MustElement("p.project-details__wrapper__container__name-area__name").MustText()
		t := &testing.T{}
		assert.Equal(t, currentProjectNameFromSideMenu, projectNameFromEditScreen)

		descriptionText := page.MustElement("p.project-details__wrapper__container__description-area__description").MustText()
		fmt.Println(descriptionText)
		nameEditButton := page.MustElement("div.container.white:nth-of-type(1)").MustText()
		descriptionEditButton := page.MustElement("#app > div > div > div.dashboard__wrap__main-area > div.dashboard__wrap__main-area__content > div.project-details > div > div > div.project-details__wrapper__container__description-area > div").MustText()
		fmt.Println(nameEditButton, descriptionEditButton)

		// Output: Project Details
		// Name
		// Description
		// No description yet. Please enter some information if any.
		// Edit Edit
	}

	func Example_projectScreen () {
		page, browser := login_to_account()
		defer browser.MustClose()

		// checking notification
		notificationBegin := page.MustElement("b.info-bar__info-area__first-value").MustText()
		fmt.Println(strings.Contains(notificationBegin, "You have used"))
		notificationMiddle := page.MustElement("span.info-bar__info-area__first-description").MustText()
		fmt.Println(notificationMiddle)
		notificationEnd := page.MustElement("span.info-bar__info-area__second-description").MustText()
		fmt.Println(notificationEnd)
		notificationLink := page.MustElement("a.info-bar__link.blue").MustAttribute("href")
		fmt.Println(*(notificationLink))

		// checking Dashboard area title
		fmt.Println(strings.Contains(page.MustElement(".dashboard-area__title").MustText(),"Dashboard"))

		// storage div
		storageHeader := page.MustElement("p.usage-area__title:nth-of-type(1)").MustText()
		fmt.Println(storageHeader)
		storageRemaining:= page.MustElement("pre.usage-area__remaining:nth-of-type(1)").MustText()
		fmt.Println(storageRemaining)
		storageUsed:= page.MustElement("pre.usage-area__limits-area__title:nth-of-type(1)").MustText()
		fmt.Println(storageUsed)
		storageUsedAmount:= page.MustElementX("(//*[@class=\"usage-area__limits-area__limits\"])[1]").MustText()
		fmt.Println(storageUsedAmount)

		// Bandwidht div
		bandwidthHeader := page.MustElementX("(//*[@class=\"usage-area__title\"])[2]").MustText()
		fmt.Println(bandwidthHeader)
		bandwidthRemaining:= page.MustElementX("(//*[@class=\"usage-area__remaining\"])[2]").MustText()
		fmt.Println(bandwidthRemaining)
		bandwidthUsed:= page.MustElementX("(//*[@class=\"usage-area__limits-area__title\"])[2]").MustText()
		fmt.Println(bandwidthUsed)
		bandwidthUsedAmount:= page.MustElementX("(//*[@class=\"usage-area__limits-area__limits\"])[2]").MustText()
		fmt.Println(bandwidthUsedAmount)

		// Details
		detilsHeader:= page.MustElement("h1.project-summary__title").MustText()
		fmt.Println(detilsHeader)
		userHeader:= page.MustElement("h1.summary-item__title:nth-of-type(1)").MustText()
		fmt.Println(userHeader)
		usersValue:= page.MustElement("p.summary-item__value").MustText()
		fmt.Println(usersValue)
		apiKeysHeader:= page.MustElementX("(//*[@class=\"summary-item__title\"])[2]").MustText()
		fmt.Println(apiKeysHeader)
		apiKeysValue:= page.MustElementX("(//*[@class=\"summary-item__value\"])[2]").MustText()
		fmt.Println(apiKeysValue)
		bucketsHeader:= page.MustElementX("(//*[@class=\"summary-item__title\"])[3]").MustText()
		fmt.Println(bucketsHeader)
		bucketsValue:= page.MustElementX("(//*[@class=\"summary-item__value\"])[3]").MustText()
		fmt.Println(bucketsValue)
		chargesHeader:= page.MustElementX("(//*[@class=\"summary-item__title\"])[4]").MustText()
		fmt.Println(chargesHeader)
		chargesValue:= page.MustElementX("(//*[@class=\"summary-item__value\"])[4]").MustText()
		fmt.Println(chargesValue)

		// project without buckets
		noBucketImage:= page.MustHas("img.no-buckets-area__image")
		fmt.Println(noBucketImage)
		noBucketImageLocation:= page.MustElement("img.no-buckets-area__image").MustAttribute("src")
		fmt.Println(*noBucketImageLocation)
		noBucketsMessage:= page.MustElement("h2.no-buckets-area__message").MustText()
		fmt.Println(noBucketsMessage)
		getStartedButtonLink:= page.MustElement("a.no-buckets-area__first-button").MustAttribute("href")
		fmt.Println(*getStartedButtonLink)
		getStartedButtonText:= page.MustElement("a.no-buckets-area__first-button").MustText()
		fmt.Println(getStartedButtonText)
		docsButtonLink:= page.MustElement("a.no-buckets-area__second-button").MustAttribute("href")
		fmt.Println(*docsButtonLink)
		docsButtonText:= page.MustElement("a.no-buckets-area__second-button").MustText()
		fmt.Println(docsButtonText)
		whycantLink:= page.MustElement("a.no-buckets-area__help").MustAttribute("href")
		fmt.Println(*whycantLink)
		whycantText:= page.MustElement("a.no-buckets-area__help").MustText()
		fmt.Println(whycantText)






		// Output: true
		// of your
		// available projects.
		// https://support.tardigrade.io/hc/en-us/requests/new?ticket_form_id=360000379291
		// true
		// Storage
		// 50.00GB Remaining
		// Storage Used
		// 0 / 50.00GB
		// Bandwidth
		// 50.00GB Remaining
		// Bandwidth Used
		// 0 / 50.00GB
		// Details
		// Users
		// 1
		// API Keys
		// 0
		// Buckets
		// 0
		// Estimated Charges
		// $0.00
		// true
		// /static/dist/img/bucket.d8cab0f6.png
		// Create your first bucket to get started.
		// https://documentation.tardigrade.io/api-reference/uplink-cli
		// Get Started
		// https://documentation.tardigrade.io/
		// Visit the Docs
		// https://support.tardigrade.io/hc/en-us/articles/360035332472-Why-can-t-I-upload-from-the-browser-
		// Why can't I upload from the browser?
	}

	func Example_APIKeysScreen () {
		page, browser := login_to_account()
		defer browser.MustClose()
		page.MustElement("a.navigation-area__item-container:nth-of-type(2)").MustClick()

		// screen without keys created
		apiKeysHeader:= page.MustElement("h1.no-api-keys-area__title").MustText()
		fmt.Println(apiKeysHeader)
		apiKeysText:= page.MustElement("p.no-api-keys-area__sub-title").MustText()
		fmt.Println(apiKeysText)
		createKeyButton:= page.MustElement("div.no-api-keys-area__button.container").MustText()
		fmt.Println(createKeyButton)
		uploadSteps:= page.MustElement("div.no-api-keys-area__steps-area__numbers").MustVisible()
		fmt.Println(uploadSteps)
		firstStepText:= page.MustElement("h2.no-api-keys-area__steps-area__items__create-api-key__title").MustText()
		fmt.Println(firstStepText)
		firstStepImage:= page.MustHas("img.no-api-keys-area-image")
		fmt.Println(firstStepImage)
		firstStepImagePath:= page.MustElement("img.no-api-keys-area-image:nth-of-type(1)").MustAttribute("src")
		fmt.Println(*firstStepImagePath)
		secondStepText:= page.MustElement("h2.no-api-keys-area__steps-area__items__setup-uplink__title").MustText()
		fmt.Println(secondStepText)
		secndStepImage:= page.MustHasX("(//*[@class=\"no-api-keys-area-image\"])[2]")
		fmt.Println(secndStepImage)
		secondStepImagePath:= page.MustElementX("(//*[@class=\"no-api-keys-area-image\"])[2]").MustAttribute("src")
		fmt.Println(*secondStepImagePath)
		thirdStepText:= page.MustElement("h2.no-api-keys-area__steps-area__items__store-data__title").MustText()
		fmt.Println(thirdStepText)
		thirdStepImage:= page.MustHasX("(//*[@class=\"no-api-keys-area-image\"])[3]")
		fmt.Println(thirdStepImage)
		thirdStepImagePath:= page.MustElementX("(//*[@class=\"no-api-keys-area-image\"])[3]").MustAttribute("src")
		fmt.Println(*thirdStepImagePath)


		// Output: Create Your First API Key
		// API keys give access to the project to create buckets, upload objects
		// Create API Key
		// true
		// Create & Save API Key
		// true
		// /static/dist/img/apiKey.981d0fef.jpg
		// Setup Uplink CLI
		// true
		// /static/dist/img/uplink.30403d68.jpg
		// Store Data
		// true
		// /static/dist/img/store.eb048f38.jpg
	}


	func Example_membersScreen () {
		page, browser := login_to_account()
		defer browser.MustClose()
		page.MustElement("a.navigation-area__item-container:nth-of-type(3)").MustClick()

		membersHeaderText := page.MustElement("h1.team-header-container__title-area__title").MustText()
		fmt.Println(membersHeaderText)
		questionMark:= page.MustHas("svg.team-header-container__title-area__info-button__image")
		fmt.Println(questionMark)
		helper:= page.MustElement("svg.team-header-container__title-area__info-button__image").MustClick().MustElementX("//*[@class=\"info__message-box__text\"]").MustText()
		fmt.Println(helper)
		addmemberButton:= page.MustElement("div.button.container").MustText()
		fmt.Println(addmemberButton)
		searchPlaceholder:= page.MustElement("input.common-search-input").MustAttribute("placeholder")
		searchSizeMin:= page.MustElement("input.common-search-input").MustAttribute("style")
		page.MustElement("input.common-search-input").MustClick().MustInput("ffwefwefhg")
		searchSizeMax:= page.MustElement("input.common-search-input").MustAttribute("style")
		fmt.Println(*searchPlaceholder)
		fmt.Println(*searchSizeMin)
		fmt.Println(*searchSizeMax)
		// Output: Project Members
		// true
		// The only project role currently available is Admin, which gives full access to the project.
		// + Add
		// Search Team Members
		// width: 56px;
		// width: 540px;
	}

	func Example_LoginScreen() {

		l := launcher.New().
			Headless(false).
			Devtools(false)
		defer l.Cleanup()
		url := l.MustLaunch()

		browser := rod.New().
			Timeout(time.Minute).
			ControlURL(url).
			Trace(true).
			Slowmotion(300 * time.Millisecond).
			MustConnect()

		// Even you forget to close, rod will close it after main process ends.
		defer browser.MustClose()

		// Timeout will be passed to all chained function calls.
		// The code will panic out if any chained call is used after the timeout.
		page := browser.Timeout(15 * time.Second).MustPage(startPage)

		// Make sure viewport is always consistent.
		page.MustSetViewport(screenWidth, screenHeigth, 1, false)
		fmt.Println(page.MustElement("svg.login-container__logo").MustVisible())
		header:= page.MustElement("h1.login-area__title-container__title").MustText()
		fmt.Println(header)
		forgotText:= page.MustElement("h3.login-area__navigation-area__nav-link__link").MustText()
		fmt.Println(forgotText)
		forgotLink:= page.MustElement("a.login-area__navigation-area__nav-link").MustAttribute("href")
		fmt.Println(*forgotLink)
		createAccButton:= page.MustElement("div.login-container__register-button").MustText()
		fmt.Println(createAccButton)
		loginButton:= page.MustElement("div.login-area__submit-area__login-button").MustText()
		fmt.Println(loginButton)
		siganture:= page.MustElement("p.login-area__info-area__signature").MustText()
		fmt.Println(siganture)
		termsText:= page.MustElement("a.login-area__info-area__terms").MustText()
		fmt.Println(termsText)
		termsLink:= page.MustElement("a.login-area__info-area__terms").MustAttribute("href")
		fmt.Println(*termsLink)
		supportText:= page.MustElement("a.login-area__info-area__help").MustText()
		fmt.Println(supportText)
		supportLink:= page.MustElement("a.login-area__info-area__help").MustAttribute("href")
		fmt.Println(*supportLink)

		// Output: true
		// Login to Storj
		// Forgot password?
		// /forgot-password
		// Create Account
		// Log In
		// Storj Labs Inc 2020.
		// Terms & Conditions
		// https://tardigrade.io/terms-of-use/
		// Support
		// mailto:support@storj.io


	}

	func Example_createAccountScreen () {
		l := launcher.New().
			Headless(false).
			Devtools(false)
		defer l.Cleanup()
		url := l.MustLaunch()

		browser := rod.New().
			Timeout(time.Minute).
			ControlURL(url).
			Trace(true).
			Slowmotion(300 * time.Millisecond).
			MustConnect()

		// Even you forget to close, rod will close it after main process ends.
		defer browser.MustClose()

		// Timeout will be passed to all chained function calls.
		// The code will panic out if any chained call is used after the timeout.
		page := browser.Timeout(15 * time.Second).MustPage(startPage)
		page.MustElement("div.login-container__register-button").MustClick()

		fmt.Println(page.MustElement("svg.register-container__logo").MustVisible())
		toLogin:= page.MustElement("div.register-container__register-button").MustText()
		fmt.Println(toLogin)
		title:= page.MustElement("h1.register-area__title-container__title").MustText()
		fmt.Println(title)
		fullNameLabel:= page.MustElementX("(//*[@class=\"label-container__label\"])[1]").MustText()
		fmt.Println(fullNameLabel)
		fmt.Println(page.MustElementX("(//*[@class=\"headerless-input\"])[1]").MustVisible())
		fullNamePlaceholder:= page.MustElementX("(//*[@class=\"headerless-input\"])[1]").MustAttribute("placeholder")
		fmt.Println(*fullNamePlaceholder)
		emailLabel:= page.MustElementX("(//*[@class=\"label-container__label\"])[2]").MustText()
		fmt.Println(emailLabel)
		fmt.Println(page.MustElementX("(//*[@class=\"headerless-input\"])[2]").MustVisible())
		emailPlaceholder:= page.MustElementX("(//*[@class=\"headerless-input\"])[2]").MustAttribute("placeholder")
		fmt.Println(*emailPlaceholder)
		passwordLabel:= page.MustElementX("(//*[@class=\"label-container__label\"])[3]").MustText()
		fmt.Println(passwordLabel)
		fmt.Println(page.MustElementX("(//*[@class=\"headerless-input password\"])[1]").MustVisible())
		passwordPlaceholder:= page.MustElementX("(//*[@class=\"headerless-input password\"])[1]").MustAttribute("placeholder")
		fmt.Println(*passwordPlaceholder)
		confirmLabel:= page.MustElementX("(//*[@class=\"label-container__label\"])[4]").MustText()
		fmt.Println(confirmLabel)
		fmt.Println(page.MustElementX("(//*[@class=\"headerless-input password\"])[2]").MustVisible())
		confirmPlaceholder:= page.MustElementX("(//*[@class=\"headerless-input password\"])[2]").MustAttribute("placeholder")
		fmt.Println(*confirmPlaceholder)
		fmt.Println(page.MustElement("span.checkmark").MustVisible())
		termsLabel:= page.MustElement("h2.register-area__submit-container__terms-area__terms-confirmation > label").MustText()
		fmt.Println(termsLabel)
		//Output: true
		// Login
		// Sign Up to Storj
		// Full Name
		// true
		// Enter Full Name
		// Email
		// true
		// Enter Email
		// Password
		// true
		// Enter Password
		// Confirm Password
		// true
		// Confirm Password
		// true
		// I agree to the
	}

	func Example_createAccountScreen2 () {
		l := launcher.New().
			Headless(false).
			Devtools(false)
		defer l.Cleanup()
		url := l.MustLaunch()

		browser := rod.New().
			Timeout(time.Minute).
			ControlURL(url).
			Trace(true).
			Slowmotion(300 * time.Millisecond).
			MustConnect()

		// Even you forget to close, rod will close it after main process ends.
		defer browser.MustClose()

		// Timeout will be passed to all chained function calls.
		// The code will panic out if any chained call is used after the timeout.
		page := browser.Timeout(15 * time.Second).MustPage(startPage)
		page.MustElement("div.login-container__register-button").MustClick()
		termsLinkText:= page.MustElement("a.register-area__submit-container__terms-area__link").MustText()
		fmt.Println(termsLinkText)
		termsLink:= page.MustElement("a.register-area__submit-container__terms-area__link").MustAttribute("href")
		fmt.Println(*termsLink)
		createButton:= page.MustElement("div#createAccountButton").MustText()
		fmt.Println(createButton)
		
		// Output: Terms & Conditions
		// https://tardigrade.io/terms-of-use/
		// Create Account
	}

	func Example_forgotPassScreen () {
		l := launcher.New().
			Headless(false).
			Devtools(false)
		defer l.Cleanup()
		url := l.MustLaunch()

		browser := rod.New().
			Timeout(time.Minute).
			ControlURL(url).
			Trace(true).
			Slowmotion(300 * time.Millisecond).
			MustConnect()

		// Even you forget to close, rod will close it after main process ends.
		defer browser.MustClose()

		// Timeout will be passed to all chained function calls.
		// The code will panic out if any chained call is used after the timeout.
		page := browser.Timeout(15 * time.Second).MustPage(startPage)
		page.MustElement("a.login-area__navigation-area__nav-link").MustClick()

		fmt.Println(page.MustElement("svg.forgot-password-container__logo").MustVisible())
		backToLoginText:= page.MustElement("div.forgot-password-container__login-button").MustText()
		fmt.Println(backToLoginText)
		header:= page.MustElement("h1.forgot-password-area__title-container__title").MustText()
		fmt.Println(header)
		text:= page.MustElement("p.forgot-password-area__info-text").MustText()
		fmt.Println(text)
		//input visibility
		fmt.Println(page.MustElement("input.headerless-input").MustVisible())
		inputPlaceholder:= page.MustElement("input.headerless-input").MustAttribute("placeholder")
		fmt.Println(*inputPlaceholder)
		resetButton:= page.MustElement("div.forgot-password-area__submit-container").MustText()
		fmt.Println(resetButton)

		// Output: true
		// Back to Login
		// Forgot Password
		// Enter your email address below and we'll get you back on track.
		// true
		// Enter Your Email
		// Reset Password
	}


	func Example_APIKeysCreationFlow() {
		page, browser := login_to_account()
		defer browser.MustClose()
		page.MustElement("a.navigation-area__item-container:nth-of-type(2)").MustClick()

		page.MustElement("div.button.container").MustClick()
		time.Sleep(1* time.Second)
		// checking elements
		fmt.Println(page.MustElement("h2.new-api-key__title").MustText())
		fmt.Println(page.MustElement("div.new-api-key__close-cross-container").MustVisible())
		fmt.Println(*page.MustElement("input.headerless-input").MustAttribute("placeholder"))
		fmt.Println(page.MustElement("span.label").MustText())
		// creation flow
		page.MustElement("input.headerless-input").MustInput("23hf4fgf57f34")
		page.MustElement("span.label").MustClick()

		fmt.Println(page.MustElement("h2.save-api-popup__title").MustText())
		fmt.Println(page.MustElement("div.save-api-popup__copy-area__key-area").MustVisible())
		fmt.Println(page.MustElement("p.save-api-popup__copy-area__copy-button").MustText())
		fmt.Println(page.MustElement("span.save-api-popup__next-step-area__label").MustText())
		fmt.Println(*page.MustElement("a.save-api-popup__next-step-area__link").MustAttribute("href"))
		fmt.Println(page.MustElement("a.save-api-popup__next-step-area__link").MustText())
		fmt.Println(page.MustElement("div.container").MustText())
		page.MustElement("p.save-api-popup__copy-area__copy-button").MustClick()
		fmt.Println(page.MustElement("p.notification-wrap__text-area__message").MustText())
		page.MustElement("div.container").MustClick()





		//Output: Name Your API Key
		// true
		// Enter API Key Name
		// Next >
		// Save Your Secret API Key! It Will Appear Only Once.
		// true
		// Copy
		// Next Step:
		// https://documentation.tardigrade.io/getting-started/uploading-your-first-object/set-up-uplink-cli
		// Set Up Uplink CLI
		// Done
		// Successfully created new api key


	}

	func TestAPIKeysCreation(t *testing.T) {
		page, browser := login_to_account()
		defer browser.MustClose()
		page.MustElement("a.navigation-area__item-container:nth-of-type(2)").MustClick()

		listBeforeAdding := len(page.MustElements("div.apikey-item-container.item-component__item"))
		page.MustElement("div.button.container").MustClick()
		time.Sleep(1 * time.Second)
		// creation flow
		page.MustElement("input.headerless-input").MustInput("23hfee4fg57f34")
		page.MustElement("span.label").MustClick()
		time.Sleep(1 * time.Second)
		page.MustElement("div.container").MustClick()
		listAfterAdding := len(page.MustElements("div.apikey-item-container.item-component__item"))
		assert.Equal(t, listAfterAdding, (listBeforeAdding + 1))

	}






