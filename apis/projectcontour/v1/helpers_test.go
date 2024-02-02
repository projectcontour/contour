// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

// This file is intended to lock in the API for the code in helpers.go
// If you change the tests in this file, you must consider whether you need
// to update the version to v2alpha1 at least.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type subConditionDetails struct {
	condType string
	reason   string
	message  string
}

func TestAddErrorConditions(t *testing.T) {
	tests := map[string]struct {
		dc            *DetailedCondition
		subconditions []subConditionDetails
		want          *DetailedCondition
	}{
		"basic error add, negative polarity": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "AnError",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "AnError",
					Status:  ConditionTrue,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"basic error add, Positive polarity": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},

		"multiple reason, multiple type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: ConditionTypeSpecError,
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SpecError",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, multiple type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: ConditionTypeSpecError,
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SpecError",
						Reason:  "TestReason",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, same type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: ConditionTypeSpecError,
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SpecError",
						Reason:  "TestReason",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple different reason, same type": {
			dc: &DetailedCondition{
				Condition: Condition{
					Type: "Valid",
				},
			},
			subconditions: []subConditionDetails{
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: ConditionTypeServiceError,
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "ErrorPresent",
					Message: "At least one error present, see Errors for details",
				},
				Errors: []SubCondition{
					{
						Type:    "ServiceError",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "ServiceError",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward error",
						Status:  ConditionTrue,
					},
				},
			},
		},
	}

	for name, tc := range tests {

		for _, cond := range tc.subconditions {
			tc.dc.AddError(cond.condType, cond.reason, cond.message)
		}

		assert.Equalf(t, tc.want, tc.dc, "Add error condition failed in test %s", name)
	}
}

func TestAddWarningConditions(t *testing.T) {
	tests := map[string]struct {
		dc            *DetailedCondition
		subconditions []subConditionDetails
		want          *DetailedCondition
	}{
		"basic warning add": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
			},
			want: &DetailedCondition{
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple reason, multiple type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, multiple type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
						Reason:  "TestReason",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"same reason, same type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
		"multiple different reason, same type": {
			dc: &DetailedCondition{},
			subconditions: []subConditionDetails{
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward warning",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward warning",
				},
			},
			want: &DetailedCondition{
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning",
						Status:  ConditionTrue,
					},
					{
						Type:    "SimpleTest",
						Reason:  "TestReason2",
						Message: "We had an extra straightforward warning",
						Status:  ConditionTrue,
					},
				},
			},
		},
	}

	for name, tc := range tests {

		for _, cond := range tc.subconditions {
			tc.dc.AddWarning(cond.condType, cond.reason, cond.message)
		}

		assert.Equalf(t, tc.want, tc.dc, "Add error condition failed in test %s", name)
	}
}

func TestGetConditionFor(t *testing.T) {
	tests := map[string]struct {
		status   HTTPProxyStatus
		condType string
		want     *DetailedCondition
	}{
		"get valid condition": {
			status: HTTPProxyStatus{
				Conditions: []DetailedCondition{
					{
						Condition: Condition{
							Type:    "Valid",
							Reason:  "valid",
							Message: "valid HTTPProxy",
							Status:  ConditionTrue,
						},
					},
					{
						Condition: Condition{
							Type:    "SomeError",
							Reason:  "ErrorOccurred",
							Message: "Some error occurred.",
							Status:  ConditionTrue,
						},
					},
				},
			},
			condType: "Valid",
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Reason:  "valid",
					Message: "valid HTTPProxy",
					Status:  ConditionTrue,
				},
			},
		},
		"get error condition": {
			status: HTTPProxyStatus{
				Conditions: []DetailedCondition{
					{
						Condition: Condition{
							Type:    "Valid",
							Reason:  "valid",
							Message: "valid HTTPProxy",
							Status:  ConditionTrue,
						},
					},
					{
						Condition: Condition{
							Type:    "SomeError",
							Reason:  "ErrorOccurred",
							Message: "Some error occurred.",
							Status:  ConditionTrue,
						},
					},
				},
			},
			condType: "SomeError",
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "SomeError",
					Reason:  "ErrorOccurred",
					Message: "Some error occurred.",
					Status:  ConditionTrue,
				},
			},
		},
		"get nonexistent condition": {
			status: HTTPProxyStatus{
				Conditions: []DetailedCondition{
					{
						Condition: Condition{
							Type:    "Valid",
							Reason:  "valid",
							Message: "valid HTTPProxy",
							Status:  ConditionTrue,
						},
					},
					{
						Condition: Condition{
							Type:    "SomeError",
							Reason:  "ErrorOccurred",
							Message: "Some error occurred.",
							Status:  ConditionTrue,
						},
					},
				},
			},
			condType: "Nonexistent",
			want:     nil,
		},
		"get from empty slice condition": {
			status:   HTTPProxyStatus{},
			condType: "Nonexistent",
			want:     nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.status.GetConditionFor(tc.condType)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetError(t *testing.T) {
	dcWithErrors := &DetailedCondition{
		Errors: []SubCondition{
			{
				Type:    "ServiceError",
				Reason:  "SimpleReason",
				Message: "We had a simple error 1",
				Status:  ConditionTrue,
			},
		},
	}

	firstSubCond := SubCondition{
		Type:    "ServiceError",
		Reason:  "SimpleReason",
		Message: "We had a simple error 1",
		Status:  ConditionTrue,
	}

	gotSubCond, ok := dcWithErrors.GetError(ConditionTypeServiceError)
	assert.True(t, ok)
	assert.Equal(t, firstSubCond, gotSubCond)

	dcEmpty := &DetailedCondition{}
	emptySubCond, ok := dcEmpty.GetError(ConditionTypeServiceError)
	assert.False(t, ok)
	assert.Equal(t, SubCondition{}, emptySubCond)
}

func TestGetWarning(t *testing.T) {
	dcWithErrors := &DetailedCondition{
		Warnings: []SubCondition{
			{
				Type:    "SimpleTest1",
				Reason:  "SimpleReason",
				Message: "We had a simple warning 1",
				Status:  ConditionTrue,
			},
			{
				Type:    "SimpleTest2",
				Reason:  "SimpleReason",
				Message: "We had a simple warning 2",
				Status:  ConditionTrue,
			},
		},
	}

	firstSubCond := SubCondition{
		Type:    "SimpleTest1",
		Reason:  "SimpleReason",
		Message: "We had a simple warning 1",
		Status:  ConditionTrue,
	}

	gotSubCond, ok := dcWithErrors.GetWarning("SimpleTest1")
	assert.True(t, ok)
	assert.Equal(t, firstSubCond, gotSubCond)

	nonExistentCond, ok := dcWithErrors.GetWarning("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, SubCondition{}, nonExistentCond)

	dcEmpty := &DetailedCondition{}
	emptySubCond, ok := dcEmpty.GetWarning("SimpleTest1")
	assert.False(t, ok)
	assert.Equal(t, SubCondition{}, emptySubCond)
}

func TestTruncateLongMessage(t *testing.T) {
	shortmessage := "This is a message shorter than the max length"

	assert.Equal(t, shortmessage, truncateLongMessage(shortmessage))

	truncatedLongMessage := longMessage[:LongMessageLength]

	assert.Equal(t, truncatedLongMessage, truncateLongMessage(longMessage))
}

// nolint:misspell
const longMessage = `It is a truth universally acknowledged, that a single man in possession of a good fortune, must be in want of a wife.

However little known the feelings or views of such a man may be on his first entering a neighbourhood, this truth is so well fixed in the minds of the surrounding families, that he is considered the rightful property of some one or other of their daughters.

“My dear Mr. Bennet,” said his lady to him one day, “have you heard that Netherfield Park is let at last?”

Mr. Bennet replied that he had not.

“But it is,” returned she; “for Mrs. Long has just been here, and she told me all about it.”

Mr. Bennet made no answer.

“Do you not want to know who has taken it?” cried his wife impatiently.

“You want to tell me, and I have no objection to hearing it.”

This was invitation enough.

“Why, my dear, you must know, Mrs. Long says that Netherfield is taken by a young man of large fortune from the north of England; that he came down on Monday in a chaise and four to see the place, and was so much delighted with it, that he agreed with Mr. Morris immediately; that he is to take possession before Michaelmas, and some of his servants are to be in the house by the end of next week.”

“What is his name?”

“Bingley.”

“Is he married or single?”

“Oh! Single, my dear, to be sure! A single man of large fortune; four or five thousand a year. What a fine thing for our girls!”

“How so? How can it affect them?”

“My dear Mr. Bennet,” replied his wife, “how can you be so tiresome! You must know that I am thinking of his marrying one of them.”

“Is that his design in settling here?”

“Design! Nonsense, how can you talk so! But it is very likely that he may fall in love with one of them, and therefore you must visit him as soon as he comes.”

“I see no occasion for that. You and the girls may go, or you may send them by themselves, which perhaps will be still better, for as you are as handsome as any of them, Mr. Bingley may like you the best of the party.”

“My dear, you flatter me. I certainly have had my share of beauty, but I do not pretend to be anything extraordinary now. When a woman has five grown-up daughters, she ought to give over thinking of her own beauty.”

“In such cases, a woman has not often much beauty to think of.”

“But, my dear, you must indeed go and see Mr. Bingley when he comes into the neighbourhood.”

“It is more than I engage for, I assure you.”

“But consider your daughters. Only think what an establishment it would be for one of them. Sir William and Lady Lucas are determined to go, merely on that account, for in general, you know, they visit no newcomers. Indeed you must go, for it will be impossible for us to visit him if you do not.”

“You are over-scrupulous, surely. I dare say Mr. Bingley will be very glad to see you; and I will send a few lines by you to assure him of my hearty consent to his marrying whichever he chooses of the girls; though I must throw in a good word for my little Lizzy.”

“I desire you will do no such thing. Lizzy is not a bit better than the others; and I am sure she is not half so handsome as Jane, nor half so good-humoured as Lydia. But you are always giving her the preference.”

“They have none of them much to recommend them,” replied he; “they are all silly and ignorant like other girls; but Lizzy has something more of quickness than her sisters.”

“Mr. Bennet, how can you abuse your own children in such a way? You take delight in vexing me. You have no compassion for my poor nerves.”

“You mistake me, my dear. I have a high respect for your nerves. They are my old friends. I have heard you mention them with consideration these last twenty years at least.”

“Ah, you do not know what I suffer.”

“But I hope you will get over it, and live to see many young men of four thousand a year come into the neighbourhood.”

“It will be no use to us, if twenty such should come, since you will not visit them.”

“Depend upon it, my dear, that when there are twenty, I will visit them all.”

Mr. Bennet was so odd a mixture of quick parts, sarcastic humour, reserve, and caprice, that the experience of three-and-twenty years had been insufficient to make his wife understand his character. Her mind was less difficult to develop. She was a woman of mean understanding, little information, and uncertain temper. When she was discontented, she fancied herself nervous. The business of her life was to get her daughters married; its solace was visiting and news.


Chapter 2
Mr. Bennet was among the earliest of those who waited on Mr. Bingley. He had always intended to visit him, though to the last always assuring his wife that he should not go; and till the evening after the visit was paid she had no knowledge of it. It was then disclosed in the following manner. Observing his second daughter employed in trimming a hat, he suddenly addressed her with:

“I hope Mr. Bingley will like it, Lizzy.”

“We are not in a way to know what Mr. Bingley likes,” said her mother resentfully, “since we are not to visit.”

“But you forget, mamma,” said Elizabeth, “that we shall meet him at the assemblies, and that Mrs. Long promised to introduce him.”

“I do not believe Mrs. Long will do any such thing. She has two nieces of her own. She is a selfish, hypocritical woman, and I have no opinion of her.”

“No more have I,” said Mr. Bennet; “and I am glad to find that you do not depend on her serving you.”

Mrs. Bennet deigned not to make any reply, but, unable to contain herself, began scolding one of her daughters.

“Don’t keep coughing so, Kitty, for Heaven’s sake! Have a little compassion on my nerves. You tear them to pieces.”

“Kitty has no discretion in her coughs,” said her father; “she times them ill.”

“I do not cough for my own amusement,” replied Kitty fretfully. “When is your next ball to be, Lizzy?”

“To-morrow fortnight.”

“Aye, so it is,” cried her mother, “and Mrs. Long does not come back till the day before; so it will be impossible for her to introduce him, for she will not know him herself.”

“Then, my dear, you may have the advantage of your friend, and introduce Mr. Bingley to her.”

“Impossible, Mr. Bennet, impossible, when I am not acquainted with him myself; how can you be so teasing?”

“I honour your circumspection. A fortnight’s acquaintance is certainly very little. One cannot know what a man really is by the end of a fortnight. But if we do not venture somebody else will; and after all, Mrs. Long and her nieces must stand their chance; and, therefore, as she will think it an act of kindness, if you decline the office, I will take it on myself.”

The girls stared at their father. Mrs. Bennet said only, “Nonsense, nonsense!”

“What can be the meaning of that emphatic exclamation?” cried he. “Do you consider the forms of introduction, and the stress that is laid on them, as nonsense? I cannot quite agree with you there. What say you, Mary? For you are a young lady of deep reflection, I know, and read great books and make extracts.”

Mary wished to say something sensible, but knew not how.

“While Mary is adjusting her ideas,” he continued, “let us return to Mr. Bingley.”

“I am sick of Mr. Bingley,” cried his wife.

“I am sorry to hear that; but why did not you tell me that before? If I had known as much this morning I certainly would not have called on him. It is very unlucky; but as I have actually paid the visit, we cannot escape the acquaintance now.”

The astonishment of the ladies was just what he wished; that of Mrs. Bennet perhaps surpassing the rest; though, when the first tumult of joy was over, she began to declare that it was what she had expected all the while.

“How good it was in you, my dear Mr. Bennet! But I knew I should persuade you at last. I was sure you loved your girls too well to neglect such an acquaintance. Well, how pleased I am! and it is such a good joke, too, that you should have gone this morning and never said a word about it till now.”

“Now, Kitty, you may cough as much as you choose,” said Mr. Bennet; and, as he spoke, he left the room, fatigued with the raptures of his wife.

“What an excellent father you have, girls!” said she, when the door was shut. “I do not know how you will ever make him amends for his kindness; or me, either, for that matter. At our time of life it is not so pleasant, I can tell you, to be making new acquaintances every day; but for your sakes, we would do anything. Lydia, my love, though you are the youngest, I dare say Mr. Bingley will dance with you at the next ball.”

“Oh!” said Lydia stoutly, “I am not afraid; for though I am the youngest, I’m the tallest.”

The rest of the evening was spent in conjecturing how soon he would return Mr. Bennet’s visit, and determining when they should ask him to dinner.


Chapter 3
Not all that Mrs. Bennet, however, with the assistance of her five daughters, could ask on the subject, was sufficient to draw from her husband any satisfactory description of Mr. Bingley. They attacked him in various ways—with barefaced questions, ingenious suppositions, and distant surmises; but he eluded the skill of them all, and they were at last obliged to accept the second-hand intelligence of their neighbour, Lady Lucas. Her report was highly favourable. Sir William had been delighted with him. He was quite young, wonderfully handsome, extremely agreeable, and, to crown the whole, he meant to be at the next assembly with a large party. Nothing could be more delightful! To be fond of dancing was a certain step towards falling in love; and very lively hopes of Mr. Bingley’s heart were entertained.

“If I can but see one of my daughters happily settled at Netherfield,” said Mrs. Bennet to her husband, “and all the others equally well married, I shall have nothing to wish for.”

In a few days Mr. Bingley returned Mr. Bennet’s visit, and sat about ten minutes with him in his library. He had entertained hopes of being admitted to a sight of the young ladies, of whose beauty he had heard much; but he saw only the father. The ladies were somewhat more fortunate, for they had the advantage of ascertaining from an upper window that he wore a blue coat, and rode a black horse.

An invitation to dinner was soon afterwards dispatched; and already had Mrs. Bennet planned the courses that were to do credit to her housekeeping, when an answer arrived which deferred it all. Mr. Bingley was obliged to be in town the following day, and, consequently, unable to accept the honour of their invitation, etc. Mrs. Bennet was quite disconcerted. She could not imagine what business he could have in town so soon after his arrival in Hertfordshire; and she began to fear that he might be always flying about from one place to another, and never settled at Netherfield as he ought to be. Lady Lucas quieted her fears a little by starting the idea of his being gone to London only to get a large party for the ball; and a report soon followed that Mr. Bingley was to bring twelve ladies and seven gentlemen with him to the assembly. The girls grieved over such a number of ladies, but were comforted the day before the ball by hearing, that instead of twelve he brought only six with him from London—his five sisters and a cousin. And when the party entered the assembly room it consisted of only five altogether—Mr. Bingley, his two sisters, the husband of the eldest, and another young man.

Mr. Bingley was good-looking and gentlemanlike; he had a pleasant countenance, and easy, unaffected manners. His sisters were fine women, with an air of decided fashion. His brother-in-law, Mr. Hurst, merely looked the gentleman; but his friend Mr. Darcy soon drew the attention of the room by his fine, tall person, handsome features, noble mien, and the report which was in general circulation within five minutes after his entrance, of his having ten thousand a year. The gentlemen pronounced him to be a fine figure of a man, the ladies declared he was much handsomer than Mr. Bingley, and he was looked at with great admiration for about half the evening, till his manners gave a disgust which turned the tide of his popularity; for he was discovered to be proud; to be above his company, and above being pleased; and not all his large estate in Derbyshire could then save him from having a most forbidding, disagreeable countenance, and being unworthy to be compared with his friend.

Mr. Bingley had soon made himself acquainted with all the principal people in the room; he was lively and unreserved, danced every dance, was angry that the ball closed so early, and talked of giving one himself at Netherfield. Such amiable qualities must speak for themselves. What a contrast between him and his friend! Mr. Darcy danced only once with Mrs. Hurst and once with Miss Bingley, declined being introduced to any other lady, and spent the rest of the evening in walking about the room, speaking occasionally to one of his own party. His character was decided. He was the proudest, most disagreeable man in the world, and everybody hoped that he would never come there again. Amongst the most violent against him was Mrs. Bennet, whose dislike of his general behaviour was sharpened into particular resentment by his having slighted one of her daughters.

Elizabeth Bennet had been obliged, by the scarcity of gentlemen, to sit down for two dances; and during part of that time, Mr. Darcy had been standing near enough for her to hear a conversation between him and Mr. Bingley, who came from the dance for a few minutes, to press his friend to join it.

“Come, Darcy,” said he, “I must have you dance. I hate to see you standing about by yourself in this stupid manner. You had much better dance.”

“I certainly shall not. You know how I detest it, unless I am particularly acquainted with my partner. At such an assembly as this it would be insupportable. Your sisters are engaged, and there is not another woman in the room whom it would not be a punishment to me to stand up with.”

“I would not be so fastidious as you are,” cried Mr. Bingley, “for a kingdom! Upon my honour, I never met with so many pleasant girls in my life as I have this evening; and there are several of them you see uncommonly pretty.”

“You are dancing with the only handsome girl in the room,” said Mr. Darcy, looking at the eldest Miss Bennet.

“Oh! She is the most beautiful creature I ever beheld! But there is one of her sisters sitting down just behind you, who is very pretty, and I dare say very agreeable. Do let me ask my partner to introduce you.”

“Which do you mean?” and turning round he looked for a moment at Elizabeth, till catching her eye, he withdrew his own and coldly said: “She is tolerable, but not handsome enough to tempt me; I am in no humour at present to give consequence to young ladies who are slighted by other men. You had better return to your partner and enjoy her smiles, for you are wasting your time with me.”

Mr. Bingley followed his advice. Mr. Darcy walked off; and Elizabeth remained with no very cordial feelings toward him. She told the story, however, with great spirit among her friends; for she had a lively, playful disposition, which delighted in anything ridiculous.

The evening altogether passed off pleasantly to the whole family. Mrs. Bennet had seen her eldest daughter much admired by the Netherfield party. Mr. Bingley had danced with her twice, and she had been distinguished by his sisters. Jane was as much gratified by this as her mother could be, though in a quieter way. Elizabeth felt Jane’s pleasure. Mary had heard herself mentioned to Miss Bingley as the most accomplished girl in the neighbourhood; and Catherine and Lydia had been fortunate enough never to be without partners, which was all that they had yet learnt to care for at a ball. They returned, therefore, in good spirits to Longbourn, the village where they lived, and of which they were the principal inhabitants. They found Mr. Bennet still up. With a book he was regardless of time; and on the present occasion he had a good deal of curiosity as to the event of an evening which had raised such splendid expectations. He had rather hoped that his wife’s views on the stranger would be disappointed; but he soon found out that he had a different story to hear.

“Oh, my dear Mr. Bennet,” as she entered the room, “we have had a most delightful evening, a most excellent ball. I wish you had been there. Jane was so admired, nothing could be like it. Everybody said how well she looked; and Mr. Bingley thought her quite beautiful, and danced with her twice! Only think of that, my dear; he actually danced with her twice! and she was the only creature in the room that he asked a second time. First of all, he asked Miss Lucas. I was so vexed to see him stand up with her! But, however, he did not admire her at all; indeed, nobody can, you know; and he seemed quite struck with Jane as she was going down the dance. So he inquired who she was, and got introduced, and asked her for the two next. Then the two third he danced with Miss King, and the two fourth with Maria Lucas, and the two fifth with Jane again, and the two sixth with Lizzy, and the Boulanger—”

“If he had had any compassion for me,” cried her husband impatiently, “he would not have danced half so much! For God’s sake, say no more of his partners. Oh that he had sprained his ankle in the first dance!”

“Oh! my dear, I am quite delighted with him. He is so excessively handsome! And his sisters are charming women. I never in my life saw anything more elegant than their dresses. I dare say the lace upon Mrs. Hurst’s gown—”

Here she was interrupted again. Mr. Bennet protested against any description of finery. She was therefore obliged to seek another branch of the subject, and related, with much bitterness of spirit and some exaggeration, the shocking rudeness of Mr. Darcy.

“But I can assure you,” she added, “that Lizzy does not lose much by not suiting his fancy; for he is a most disagreeable, horrid man, not at all worth pleasing. So high and so conceited that there was no enduring him! He walked here, and he walked there, fancying himself so very great! Not handsome enough to dance with! I wish you had been there, my dear, to have given him one of your set-downs. I quite detest the man.”


Chapter 4
When Jane and Elizabeth were alone, the former, who had been cautious in her praise of Mr. Bingley before, expressed to her sister just how very much she admired him.

“He is just what a young man ought to be,” said she, “sensible, good-humoured, lively; and I never saw such happy manners!—so much ease, with such perfect good breeding!”

“He is also handsome,” replied Elizabeth, “which a young man ought likewise to be, if he possibly can. His character is thereby complete.”

“I was very much flattered by his asking me to dance a second time. I did not expect such a compliment.”

“Did not you? I did for you. But that is one great difference between us. Compliments always take you by surprise, and me never. What could be more natural than his asking you again? He could not help seeing that you were about five times as pretty as every other woman in the room. No thanks to his gallantry for that. Well, he certainly is very agreeable, and I give you leave to like him. You have liked many a stupider person.”

“Dear Lizzy!”

“Oh! you are a great deal too apt, you know, to like people in general. You never see a fault in anybody. All the world are good and agreeable in your eyes. I never heard you speak ill of a human being in your life.”

“I would not wish to be hasty in censuring anyone; but I always speak what I think.”

“I know you do; and it is that which makes the wonder. With your good sense, to be so honestly blind to the follies and nonsense of others! Affectation of candour is common enough—one meets with it everywhere. But to be candid without ostentation or design—to take the good of everybody’s character and make it still better, and say nothing of the bad—belongs to you alone. And so you like this man’s sisters, too, do you? Their manners are not equal to his.”

“Certainly not—at first. But they are very pleasing women when you converse with them. Miss Bingley is to live with her brother, and keep his house; and I am much mistaken if we shall not find a very charming neighbour in her.”

Elizabeth listened in silence, but was not convinced; their behaviour at the assembly had not been calculated to please in general; and with more quickness of observation and less pliancy of temper than her sister, and with a judgement too unassailed by any attention to herself, she was very little disposed to approve them. They were in fact very fine ladies; not deficient in good humour when they were pleased, nor in the power of making themselves agreeable when they chose it, but proud and conceited. They were rather handsome, had been educated in one of the first private seminaries in town, had a fortune of twenty thousand pounds, were in the habit of spending more than they ought, and of associating with people of rank, and were therefore in every respect entitled to think well of themselves, and meanly of others. They were of a respectable family in the north of England; a circumstance more deeply impressed on their memories than that their brother’s fortune and their own had been acquired by trade.

Mr. Bingley inherited property to the amount of nearly a hundred thousand pounds from his father, who had intended to purchase an estate, but did not live to do it. Mr. Bingley intended it likewise, and sometimes made choice of his county; but as he was now provided with a good house and the liberty of a manor, it was doubtful to many of those who best knew the easiness of his temper, whether he might not spend the remainder of his days at Netherfield, and leave the next generation to purchase.

His sisters were anxious for his having an estate of his own; but, though he was now only established as a tenant, Miss Bingley was by no means unwilling to preside at his table—nor was Mrs. Hurst, who had married a man of more fashion than fortune, less disposed to consider his house as her home when it suited her. Mr. Bingley had not been of age two years, when he was tempted by an accidental recommendation to look at Netherfield House. He did look at it, and into it for half-an-hour—was pleased with the situation and the principal rooms, satisfied with what the owner said in its praise, and took it immediately.

Between him and Darcy there was a very steady friendship, in spite of great opposition of character. Bingley was endeared to Darcy by the easiness, openness, and ductility of his temper, though no disposition could offer a greater contrast to his own, and though with his own he never appeared dissatisfied. On the strength of Darcy’s regard, Bingley had the firmest reliance, and of his judgement the highest opinion. In understanding, Darcy was the superior. Bingley was by no means deficient, but Darcy was clever. He was at the same time haughty, reserved, and fastidious, and his manners, though well-bred, were not inviting. In that respect his friend had greatly the advantage. Bingley was sure of being liked wherever he appeared, Darcy was continually giving offense.

The manner in which they spoke of the Meryton assembly was sufficiently characteristic. Bingley had never met with more pleasant people or prettier girls in his life; everybody had been most kind and attentive to him; there had been no formality, no stiffness; he had soon felt acquainted with all the room; and, as to Miss Bennet, he could not conceive an angel more beautiful. Darcy, on the contrary, had seen a collection of people in whom there was little beauty and no fashion, for none of whom he had felt the smallest interest, and from none received either attention or pleasure. Miss Bennet he acknowledged to be pretty, but she smiled too much.

Mrs. Hurst and her sister allowed it to be so—but still they admired her and liked her, and pronounced her to be a sweet girl, and one whom they would not object to know more of. Miss Bennet was therefore established as a sweet girl, and their brother felt authorized by such commendation to think of her as he chose.


Chapter 5
Within a short walk of Longbourn lived a family with whom the Bennets were particularly intimate. Sir William Lucas had been formerly in trade in Meryton, where he had made a tolerable fortune, and risen to the honour of knighthood by an address to the king during his mayoralty. The distinction had perhaps been felt too strongly. It had given him a disgust to his business, and to his residence in a small market town; and, in quitting them both, he had removed with his family to a house about a mile from Meryton, denominated from that period Lucas Lodge, where he could think with pleasure of his own importance, and, unshackled by business, occupy himself solely in being civil to all the world. For, though elated by his rank, it did not render him supercilious; on the contrary, he was all attention to everybody. By nature inoffensive, friendly, and obliging, his presentation at St. James’s had made him courteous.

Lady Lucas was a very good kind of woman, not too clever to be a valuable neighbour to Mrs. Bennet. They had several children. The eldest of them, a sensible, intelligent young woman, about twenty-seven, was Elizabeth’s intimate friend.

That the Miss Lucases and the Miss Bennets should meet to talk over a ball was absolutely necessary; and the morning after the assembly brought the former to Longbourn to hear and to communicate.

“You began the evening well, Charlotte,” said Mrs. Bennet with civil self-command to Miss Lucas. “You were Mr. Bingley’s first choice.”

“Yes; but he seemed to like his second better.”

“Oh! you mean Jane, I suppose, because he danced with her twice. To be sure that did seem as if he admired her—indeed I rather believe he did—I heard something about it—but I hardly know what—something about Mr. Robinson.”

“Perhaps you mean what I overheard between him and Mr. Robinson; did not I mention it to you? Mr. Robinson’s asking him how he liked our Meryton assemblies, and whether he did not think there were a great many pretty women in the room, and which he thought the prettiest? and his answering immediately to the last question: ‘Oh! the eldest Miss Bennet, beyond a doubt; there cannot be two opinions on that point.’”

“Upon my word! Well, that is very decided indeed—that does seem as if—but, however, it may all come to nothing, you know.”

“My overhearings were more to the purpose than yours, Eliza,” said Charlotte. “Mr. Darcy is not so well worth listening to as his friend, is he?—poor Eliza!—to be only just tolerable.”

“I beg you would not put it into Lizzy’s head to be vexed by his ill-treatment, for he is such a disagreeable man, that it would be quite a misfortune to be liked by him. Mrs. Long told me last night that he sat close to her for half-an-hour without once opening his lips.”

“Are you quite sure, ma’am?—is not there a little mistake?” said Jane. “I certainly saw Mr. Darcy speaking to her.”

“Aye—because she asked him at last how he liked Netherfield, and he could not help answering her; but she said he seemed quite angry at being spoke to.”

“Miss Bingley told me,” said Jane, “that he never speaks much, unless among his intimate acquaintances. With them he is remarkably agreeable.”

“I do not believe a word of it, my dear. If he had been so very agreeable, he would have talked to Mrs. Long. But I can guess how it was; everybody says that he is eat up with pride, and I dare say he had heard somehow that Mrs. Long does not keep a carriage, and had come to the ball in a hack chaise.”

“I do not mind his not talking to Mrs. Long,” said Miss Lucas, “but I wish he had danced with Eliza.”

“Another time, Lizzy,” said her mother, “I would not dance with him, if I were you.”

“I believe, ma’am, I may safely promise you never to dance with him.”

“His pride,” said Miss Lucas, “does not offend me so much as pride often does, because there is an excuse for it. One cannot wonder that so very fine a young man, with family, fortune, everything in his favour, should think highly of himself. If I may so express it, he has a right to be proud.”

“That is very true,” replied Elizabeth, “and I could easily forgive his pride, if he had not mortified mine.”

“Pride,” observed Mary, who piqued herself upon the solidity of her reflections, “is a very common failing, I believe. By all that I have ever read, I am convinced that it is very common indeed; that human nature is particularly prone to it, and that there are very few of us who do not cherish a feeling of self-complacency on the score of some quality or other, real or imaginary. Vanity and pride are different things, though the words are often used synonymously. A person may be proud without being vain. Pride relates more to our opinion of ourselves, vanity to what we would have others think of us.”

“If I were as rich as Mr. Darcy,” cried a young Lucas, who came with his sisters, “I should not care how proud I was. I would keep a pack of foxhounds, and drink a bottle of wine a day.”

“Then you would drink a great deal more than you ought,” said Mrs. Bennet; “and if I were to see you at it, I should take away your bottle directly.”

The boy protested that she should not; she continued to declare that she would, and the argument ended only with the visit.


Chapter 6
The ladies of Longbourn soon waited on those of Netherfield. The visit was soon returned in due form. Miss Bennet’s pleasing manners grew on the goodwill of Mrs. Hurst and Miss Bingley; and though the mother was found to be intolerable, and the younger sisters not worth speaking to, a wish of being better acquainted with them was expressed towards the two eldest. By Jane, this attention was received with the greatest pleasure, but Elizabeth still saw superciliousness in their treatment of everybody, hardly excepting even her sister, and could not like them; though their kindness to Jane, such as it was, had a value as arising in all probability from the influence of their brother’s admiration. It was generally evident whenever they met, that he did admire her and to her it was equally evident that Jane was yielding to the preference which she had begun to entertain for him from the first, and was in a way to be very much in love; but she considered with pleasure that it was not likely to be discovered by the world in general, since Jane united, with great strength of feeling, a composure of temper and a uniform cheerfulness of manner which would guard her from the suspicions of the impertinent. She mentioned this to her friend Miss Lucas.

“It may perhaps be pleasant,” replied Charlotte, “to be able to impose on the public in such a case; but it is sometimes a disadvantage to be so very guarded. If a woman conceals her affection with the same skill from the object of it, she may lose the opportunity of fixing him; and it will then be but poor consolation to believe the world equally in the dark. There is so much of gratitude or vanity in almost every attachment, that it is not safe to leave any to itself. We can all begin freely—a slight preference is natural enough; but there are very few of us who have heart enough to be really in love without encouragement. In nine cases out of ten a woman had better show more affection than she feels. Bingley likes your sister undoubtedly; but he may never do more than like her, if she does not help him on.”

“But she does help him on, as much as her nature will allow. If I can perceive her regard for him, he must be a simpleton, indeed, not to discover it too.”

“Remember, Eliza, that he does not know Jane’s disposition as you do.”

“But if a woman is partial to a man, and does not endeavour to conceal it, he must find it out.”

“Perhaps he must, if he sees enough of her. But, though Bingley and Jane meet tolerably often, it is never for many hours together; and, as they always see each other in large mixed parties, it is impossible that every moment should be employed in conversing together. Jane should therefore make the most of every half-hour in which she can command his attention. When she is secure of him, there will be more leisure for falling in love as much as she chooses.”

“Your plan is a good one,” replied Elizabeth, “where nothing is in question but the desire of being well married, and if I were determined to get a rich husband, or any husband, I dare say I should adopt it. But these are not Jane’s feelings; she is not acting by design. As yet, she cannot even be certain of the degree of her own regard nor of its reasonableness. She has known him only a fortnight. She danced four dances with him at Meryton; she saw him one morning at his own house, and has since dined with him in company four times. This is not quite enough to make her understand his character.”

“Not as you represent it. Had she merely dined with him, she might only have discovered whether he had a good appetite; but you must remember that four evenings have also been spent together—and four evenings may do a great deal.”

“Yes; these four evenings have enabled them to ascertain that they both like Vingt-un better than Commerce; but with respect to any other leading characteristic, I do not imagine that much has been unfolded.”

“Well,” said Charlotte, “I wish Jane success with all my heart; and if she were married to him to-morrow, I should think she had as good a chance of happiness as if she were to be studying his character for a twelvemonth. Happiness in marriage is entirely a matter of chance. If the dispositions of the parties are ever so well known to each other or ever so similar beforehand, it does not advance their felicity in the least. They always continue to grow sufficiently unlike afterwards to have their share of vexation; and it is better to know as little as possible of the defects of the person with whom you are to pass your life.”

“You make me laugh, Charlotte; but it is not sound. You know it is not sound, and that you would never act in this way yourself.”

Occupied in observing Mr. Bingley’s attentions to her sister, Elizabeth was far from suspecting that she was herself becoming an object of some interest in the eyes of his friend. Mr. Darcy had at first scarcely allowed her to be pretty; he had looked at her without admiration at the ball; and when they next met, he looked at her only to criticise. But no sooner had he made it clear to himself and his friends that she hardly had a good feature in her face, than he began to find it was rendered uncommonly intelligent by the beautiful expression of her dark eyes. To this discovery succeeded some others equally mortifying. Though he had detected with a critical eye more than one failure of perfect symmetry in her form, he was forced to acknowledge her figure to be light and pleasing; and in spite of his asserting that her manners were not those of the fashionable world, he was caught by their easy playfulness. Of this she was perfectly unaware; to her he was only the man who made himself agreeable nowhere, and who had not thought her handsome enough to dance with.

He began to wish to know more of her, and as a step towards conversing with her himself, attended to her conversation with others. His doing so drew her notice. It was at Sir William Lucas’s, where a large party were assembled.

“What does Mr. Darcy mean,” said she to Charlotte, “by listening to my conversation with Colonel Forster?”

“That is a question which Mr. Darcy only can answer.”

“But if he does it any more I shall certainly let him know that I see what he is about. He has a very satirical eye, and if I do not begin by being impertinent myself, I shall soon grow afraid of him.”

On his approaching them soon afterwards, though without seeming to have any intention of speaking, Miss Lucas defied her friend to mention such a subject to him; which immediately provoking Elizabeth to do it, she turned to him and said:

“Did you not think, Mr. Darcy, that I expressed myself uncommonly well just now, when I was teasing Colonel Forster to give us a ball at Meryton?”

“With great energy; but it is always a subject which makes a lady energetic.”

“You are severe on us.”

“It will be her turn soon to be teased,” said Miss Lucas. “I am going to open the instrument, Eliza, and you know what follows.”

“You are a very strange creature by way of a friend!—always wanting me to play and sing before anybody and everybody! If my vanity had taken a musical turn, you would have been invaluable; but as it is, I would really rather not sit down before those who must be in the habit of hearing the very best performers.” On Miss Lucas’s persevering, however, she added, “Very well, if it must be so, it must.” And gravely glancing at Mr. Darcy, “There is a fine old saying, which everybody here is of course familiar with: ‘Keep your breath to cool your porridge’; and I shall keep mine to swell my song.”

Her performance was pleasing, though by no means capital. After a song or two, and before she could reply to the entreaties of several that she would sing again, she was eagerly succeeded at the instrument by her sister Mary, who having, in consequence of being the only plain one in the family, worked hard for knowledge and accomplishments, was always impatient for display.

Mary had neither genius nor taste; and though vanity had given her application, it had given her likewise a pedantic air and conceited manner, which would have injured a higher degree of excellence than she had reached. Elizabeth, easy and unaffected, had been listened to with much more pleasure, though not playing half so well; and Mary, at the end of a long concerto, was glad to purchase praise and gratitude by Scotch and Irish airs, at the request of her younger sisters, who, with some of the Lucases, and two or three officers, joined eagerly in dancing at one end of the room.

Mr. Darcy stood near them in silent indignation at such a mode of passing the evening, to the exclusion of all conversation, and was too much engrossed by his thoughts to perceive that Sir William Lucas was his neighbour, till Sir William thus began:

“What a charming amusement for young people this is, Mr. Darcy! There is nothing like dancing after all. I consider it as one of the first refinements of polished society.”

“Certainly, sir; and it has the advantage also of being in vogue amongst the less polished societies of the world. Every savage can dance.”

Sir William only smiled. “Your friend performs delightfully,” he continued after a pause, on seeing Bingley join the group; “and I doubt not that you are an adept in the science yourself, Mr. Darcy.”

“You saw me dance at Meryton, I believe, sir.”

“Yes, indeed, and received no inconsiderable pleasure from the sight. Do you often dance at St. James’s?”

“Never, sir.”

“Do you not think it would be a proper compliment to the place?”

“It is a compliment which I never pay to any place if I can avoid it.”

“You have a house in town, I conclude?”

Mr. Darcy bowed.

“I had once had some thought of fixing in town myself—for I am fond of superior society; but I did not feel quite certain that the air of London would agree with Lady Lucas.”

He paused in hopes of an answer; but his companion was not disposed to make any; and Elizabeth at that instant moving towards them, he was struck with the action of doing a very gallant thing, and called out to her:

“My dear Miss Eliza, why are you not dancing? Mr. Darcy, you must allow me to present this young lady to you as a very desirable partner. You cannot refuse to dance, I am sure when so much beauty is before you.” And, taking her hand, he would have given it to Mr. Darcy who, though extremely surprised, was not unwilling to receive it, when she instantly drew back, and said with some discomposure to Sir William:

“Indeed, sir, I have not the least intention of dancing. I entreat you not to suppose that I moved this way in order to beg for a partner.”

Mr. Darcy, with grave propriety, requested to be allowed the honour of her hand, but in vain. Elizabeth was determined; nor did Sir William at all shake her purpose by his attempt at persuasion.

“You excel so much in the dance, Miss Eliza, that it is cruel to deny me the happiness of seeing you; and though this gentleman dislikes the amusement in general, he can have no objection, I am sure, to oblige us for one half-hour.”

“Mr. Darcy is all politeness,” said Elizabeth, smiling.

“He is, indeed; but, considering the inducement, my dear Miss Eliza, we cannot wonder at his complaisance—for who would object to such a partner?”

Elizabeth looked archly, and turned away. Her resistance had not injured her with the gentleman, and he was thinking of her with some complacency, when thus accosted by Miss Bingley:

“I can guess the subject of your reverie.”

“I should imagine not.”

“You are considering how insupportable it would be to pass many evenings in this manner—in such society; and indeed I am quite of your opinion. I was never more annoyed! The insipidity, and yet the noise—the nothingness, and yet the self-importance of all those people! What would I give to hear your strictures on them!”

“Your conjecture is totally wrong, I assure you. My mind was more agreeably engaged. I have been meditating on the very great pleasure which a pair of fine eyes in the face of a pretty woman can bestow.”

Miss Bingley immediately fixed her eyes on his face, and desired he would tell her what lady had the credit of inspiring such reflections. Mr. Darcy replied with great intrepidity:

“Miss Elizabeth Bennet.”

“Miss Elizabeth Bennet!” repeated Miss Bingley. “I am all astonishment. How long has she been such a favourite?—and pray, when am I to wish you joy?”

“That is exactly the question which I expected you to ask. A lady’s imagination is very rapid; it jumps from admiration to love, from love to matrimony, in a moment. I knew you would be wishing me joy.”

“Nay, if you are serious about it, I shall consider the matter is absolutely settled. You will be having a charming mother-in-law, indeed; and, of course, she will always be at Pemberley with you.”

He listened to her with perfect indifference while she chose to entertain herself in this manner; and as his composure convinced her that all was safe, her wit flowed long.
`
