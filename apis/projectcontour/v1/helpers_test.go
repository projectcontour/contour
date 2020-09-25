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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "AnError",
					Status:  ConditionTrue,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SecondTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error",
						Status:  ConditionTrue,
					},
					{
						Type:    "SecondTest",
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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward error, We had an extra straightforward error",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward error, We had an extra straightforward error",
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
					condType: "SimpleTest",
					reason:   "TestReason",
					message:  "We had a straightforward error",
				},
				{
					condType: "SimpleTest",
					reason:   "TestReason2",
					message:  "We had an extra straightforward error",
				},
			},
			want: &DetailedCondition{
				Condition: Condition{
					Type:    "Valid",
					Status:  ConditionFalse,
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
				Errors: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "MultipleReasons",
						Message: "We had a straightforward error, We had an extra straightforward error",
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
				Condition: Condition{
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward warning",
				},
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
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
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
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
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
				Condition: Condition{
					Reason:  "SimpleTestTestReason",
					Message: "We had a straightforward warning, We had an extra straightforward warning",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "TestReason",
						Message: "We had a straightforward warning, We had an extra straightforward warning",
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
				Condition: Condition{
					Reason:  "MultipleProblems",
					Message: "Multiple problems were found, see errors or warnings for details",
				},
				Warnings: []SubCondition{
					{
						Type:    "SimpleTest",
						Reason:  "MultipleReasons",
						Message: "We had a straightforward warning, We had an extra straightforward warning",
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

func TestGetConditionIndex(t *testing.T) {
	tests := map[string]struct {
		dcs      []DetailedCondition
		condType string
		want     int
	}{
		"get valid condition": {
			dcs: []DetailedCondition{
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
			condType: "Valid",
			want:     0,
		},
		"get error condition": {
			dcs: []DetailedCondition{
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
			condType: "SomeError",
			want:     1,
		},
		"get nonexistent condition": {
			dcs: []DetailedCondition{
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
			condType: "Nonexistent",
			want:     -1,
		},
		"get from empty slice condition": {
			dcs:      []DetailedCondition{},
			condType: "Nonexistent",
			want:     -1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := GetConditionIndex(tc.condType, tc.dcs)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetError(t *testing.T) {

	dcWithErrors := &DetailedCondition{
		Errors: []SubCondition{
			{
				Type:    "SimpleTest1",
				Reason:  "SimpleReason",
				Message: "We had a simple error 1",
				Status:  ConditionTrue,
			},
			{
				Type:    "SimpleTest2",
				Reason:  "SimpleReason",
				Message: "We had a simple error 2",
				Status:  ConditionTrue,
			},
		},
	}

	firstSubCond := SubCondition{
		Type:    "SimpleTest1",
		Reason:  "SimpleReason",
		Message: "We had a simple error 1",
		Status:  ConditionTrue,
	}

	gotSubCond, ok := dcWithErrors.GetError("SimpleTest1")
	assert.True(t, ok)
	assert.Equal(t, firstSubCond, gotSubCond)

	nonExistentCond, ok := dcWithErrors.GetError("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, SubCondition{}, nonExistentCond)

	dcEmpty := &DetailedCondition{}
	emptySubCond, ok := dcEmpty.GetError("SimpleTest1")
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
const longMessage = `In a hole in the ground there lived a hobbit. Not a nasty, dirty, wet hole, filled 
with the ends of worms and an oozy smell, nor yet a dry, bare, sandy hole with 
nothing in it to sit down on or to eat: it was a hobbit-hole, and that means 
comfort. 
 
It had a perfectly round door like a porthole, painted green, with a shiny 
yellow brass knob in the exact middle. The door opened on to a tube-shaped hall 
like a tunnel: a very comfortable tunnel without smoke, with panelled walls, and 
floors tiled and carpeted, provided with polished chairs, and lots and lots of pegs 
for hats and coats—the hobbit was fond of visitors. The tunnel wound on and on, 
going fairly but not quite straight into the side of the hill—The Hill, as all the 
people for many miles round called it—and many little round doors opened out 
of it, first on one side and then on another. No going upstairs for the hobbit: 
bedrooms, bathrooms, cellars, pantries (lots of these), wardrobes (he had whole 
rooms devoted to clothes), kitchens, dining-rooms, all were on the same floor, 
and indeed on the same passage. The best rooms were all on the left-hand side 
(going in), for these were the only ones to have windows, deep-set round 
windows looking over his garden, and meadows beyond, sloping down to the 
river. 
 
This hobbit was a very well-to-do hobbit, and his name was Baggins. The 
Bagginses had lived in the neighbourhood of The Hill for time out of mind, and 
people considered them very respectable, not only because most of them were 
rich, but also because they never had any adventures or did anything unexpected: 
you could tell what a Baggins would say on any question without the bother of 
asking him. This is a story of how a Baggins had an adventure, and found 
himself doing and saying things altogether unexpected. He may have lost the 
 
 
 
 
neighbours’ respect, but he gained—well, you will see whether he gained 
anything in the end. 
 
The mother of our particular hobbit—what is a hobbit? I suppose hobbits 
need some description nowadays, since they have become rare and shy of the 
Big People, as they call us. They are (or were) a little people, about half our 
height, and smaller than the bearded Dwarves. Hobbits have no beards. There is 
little or no magic about them, except the ordinary everyday sort which helps 
them to disappear quietly and quickly when large stupid folk like you and me 
come blundering along, making a noise like elephants which they can hear a 
mile off. They are inclined to be fat in the stomach; they dress in bright colours 
(chiefly green and yellow); wear no shoes, because their feet grow natural 
leathery soles and thick warm brown hair like the stuff on their heads (which is 
curly); have long clever brown fingers, good-natured faces, and laugh deep 
fruity laughs (especially after dinner, which they have twice a day when they can 
get it). Now you know enough to go on with. As I was saying, the mother of this 
hobbit—of Bilbo Baggins, that is—was the famous Belladonna Took, one of the 
three remarkable daughters of the Old Took, head of the hobbits who lived 
across The Water, the small river that ran at the foot of The Hill. It was often said 
(in other families) that long ago one of the Took ancestors must have taken a 
fairy wife. That was, of course, absurd, but certainly there was still something 
not entirely hobbitlike about them, and once in a while members of the Took- 
clan would go and have adventures. They discreetly disappeared, and the family 
hushed it up; but the fact remained that the Tooks were not as respectable as the 
Bagginses, though they were undoubtedly richer. 
 
Not that Belladonna Took ever had any adventures after she became Mrs. 
Bungo Baggins. Bungo, that was Bilbo’s father, built the most luxurious hobbit- 
hole for her (and partly with her money) that was to be found either under The 
Hill or over The Hill or across The Water, and there they remained to the end of 
their days. Still it is probable that Bilbo, her only son, although he looked and 
behaved exactly like a second edition of his solid and comfortable father, got 
something a bit queer in his make-up from the Took side, something that only 
waited for a chance to come out. The chance never arrived, until Bilbo Baggins 
was grown up, being about fifty years old or so, and living in the beautiful 
hobbit-hole built by his father, which I have just described for you, until he had 
in fact apparently settled down immovably. 
 
By some curious chance one morning long ago in the quiet of the world, 
when there was less noise and more green, and the hobbits were still numerous 
 
 
 
and prosperous, and Bilbo Baggins was standing at his door after breakfast 
smoking an enormous long wooden pipe that reached nearly down to his woolly 
toes (neatly brushed)—Gandalf came by. Gandalf! If you had heard only a 
quarter of what I have heard about him, and I have only heard very little of all 
there is to hear, you would be prepared for any sort of remarkable tale. Tales and 
adventures sprouted up all over the place wherever he went, in the most 
extraordinary fashion. He had not been down that way under The Hill for ages 
and ages, not since his friend the Old Took died, in fact, and the hobbits had 
almost forgotten what he looked like. He had been away over The Hill and 
across The Water on businesses of his own since they were all small hobbit-boys 
and hobbit-girls. 
 
All that the unsuspecting Bilbo saw that morning was an old man with a staff. 
He had a tall pointed blue hat, a long grey cloak, a silver scarf over which his 
long white beard hung down below his waist, and immense black boots. 
 
“Good Morning!” said Bilbo, and he meant it. The sun was shining, and the 
grass was very green. But Gandalf looked at him from under long bushy 
eyebrows that stuck out further than the brim of his shady hat. 
 
“What do you mean?” he said. “Do you wish me a good morning, or mean 
that it is a good morning whether I want it or not; or that you feel good this 
morning; or that it is a morning to be good on?” 
 
“All of them at once,” said Bilbo. “And a very fine morning for a pipe of 
tobacco out of doors, into the bargain. If you have a pipe about you, sit down and 
have a fill of mine! There’s no hurry, we have all the day before us!” Then Bilbo 
sat down on a seat by his door, crossed his legs, and blew out a beautiful grey 
ring of smoke that sailed up into the air without breaking and floated away over 
The Hill. 
 
“Very pretty!” said Gandalf. “But I have no time to blow smoke-rings this 
morning. I am looking for someone to share in an adventure that I am arranging, 
and it’s very difficult to find anyone.” 
 
“I should think so—in these parts! We are plain quiet folk and have no use 
for adventures. Nasty disturbing uncomfortable things! Make you late for 
dinner! I can’t think what anybody sees in them,” said our Mr. Baggins, and 
stuck one thumb behind his braces, and blew out another even bigger smoke¬ 
ring. Then he took out his morning letters, and began to read, pretending to take 
no more notice of the old man. He had decided that he was not quite his sort, and 
wanted him to go away. But the old man did not move. He stood leaning on his 
stick and gazing at the hobbit without saying anything, till Bilbo got quite 
 
 
 
uncomfortable and even a little cross. 
 
“Good morning!” he said at last. “We don’t want any adventures here, thank 
you! You might try over The Hill or across The Water.” By this he meant that the 
conversation was at an end. 
 
“What a lot of things you do use Good morning for!” said Gandalf. “Now 
you mean that you want to get rid of me, and that it won’t be good till I move 
off.” 
 
“Not at all, not at all, my dear sir! Let me see, I don’t think I know your 
name?” 
 
“Yes, yes, my dear sir—and I do know your name, Mr. Bilbo Baggins. And 
you do know my name, though you don’t remember that I belong to it. I am 
Gandalf, and Gandalf means me! To think that I should have lived to be good- 
morninged by Belladonna Took’s son, as if I was selling buttons at the door!” 
 
“Gandalf, Gandalf! Good gracious me! Not the wandering wizard that gave 
Old Took a pair of magic diamond studs that fastened themselves and never 
came undone till ordered? Not the fellow who used to tell such wonderful tales 
at parties, about dragons and goblins and giants and the rescue of princesses and 
the unexpected luck of widows’ sons? Not the man that used to make such 
particularly excellent fireworks! I remember those! Old Took used to have them 
on Midsummer’s Eve. Splendid! They used to go up like great lilies and 
snapdragons and laburnums of fire and hang in the twilight all evening!” You 
will notice already that Mr. Baggins was not quite so prosy as he liked to 
believe, also that he was very fond of flowers. “Dear me!” he went on. “Not the 
Gandalf who was responsible for so many quiet lads and lasses going off into the 
Blue for mad adventures? Anything from climbing trees to visiting elves—or 
sailing in ships, sailing to other shores! Bless me, life used to be quite inter—I 
mean, you used to upset things badly in these parts once upon a time. I beg your 
pardon, but I had no idea you were still in business.” 
 
“Where else should I be?” said the wizard. “All the same I am pleased to find 
you remember something about me. You seem to remember my fireworks 
kindly, at any rate, and that is not without hope. Indeed for your old grandfather 
Took’s sake, and for the sake of poor Belladonna, I will give you what you asked 
for.” 
 
“I beg your pardon, I haven’t asked for anything!” 
 
“Yes, you have! Twice now. My pardon. I give it you. In fact I will go so far 
as to send you on this adventure. Very amusing for me, very good for you—and 
profitable too, very likely, if you ever get over it.” 
 
 
 
“Sorry! I don’t want any adventures, thank you. Not today. Good morning! 
But please come to tea—any time you like! Why not tomorrow? Come 
tomorrow! Good bye!” With that the hobbit turned and scuttled inside his round 
green door, and shut it as quickly as he dared, not to seem rude. Wizards after all 
are wizards. 
 
“What on earth did I ask him to tea for!” he said to himself, as he went to the 
pantry. He had only just had breakfast, but he thought a cake or two and a drink 
of something would do him good after his fright. 
 
Gandalf in the meantime was still standing outside the door, and laughing 
long but quietly. After a while he stepped up, and with the spike on his staff 
scratched a queer sign on the hobbit’s beautiful green front-door. Then he strode 
away, just about the time when Bilbo was finishing his second cake and 
beginning to think that he had escaped adventures very well. 
 
The next day he had almost forgotten about Gandalf. He did not remember 
things very well, unless he put them down on his Engagement Tablet: like this: 
Gandalf Tea Wednesday. Yesterday he had been too flustered to do anything of 
the kind. 
 
Just before tea-time there came a tremendous ring on the front-door bell, and 
then he remembered! He rushed and put on the kettle, and put out another cup 
and saucer, and an extra cake or two, and ran to the door. 
 
“I am so sorry to keep you waiting!” he was going to say, when he saw that it 
was not Gandalf at all. It was a dwarf with a blue beard tucked into a golden 
belt, and very bright eyes under his dark-green hood. As soon as the door was 
opened, he pushed inside, just as if he had been expected. 
 
He hung his hooded cloak on the nearest peg, and “Dwalin at your service!” 
he said with a low bow. 
 
“Bilbo Baggins at yours!” said the hobbit, too surprised to ask any questions 
for the moment. When the silence that followed had become uncomfortable, he 
added: “I am just about to take tea; pray come and have some with me.” A little 
stiff perhaps, but he meant it kindly. And what would you do, if an uninvited 
dwarf came and hung his things up in your hall without a word of explanation? 
 
They had not been at table long, in fact they had hardly reached the third 
cake, when there came another even louder ring at the bell. 
 
“Excuse me! ” said the hobbit, and off he went to the door. 
 
“So you have got here at last!” That was what he was going to say to Gandalf 
this time. But it was not Gandalf. Instead there was a very old-looking dwarf on 
the step with a white beard and a scarlet hood; and he too hopped inside as soon 
 
 
 
as the door was open, just as if he had been invited. 
 
“I see they have begun to arrive already,” he said when he caught sight of 
Dwalin’s green hood hanging up. He hung his red one next to it, and “Balin at 
your service!” he said with his hand on his breast. 
 
“Thank you!” said Bilbo with a gasp. It was not the correct thing to say, but 
they have begun to arrive had flustered him badly. He liked visitors, but he liked 
to know them before they arrived, and he preferred to ask them himself. He had 
a horrible thought that the cakes might run short, and then he—as the host: he 
knew his duty and stuck to it however painful—he might have to go without. 
 
“Come along in, and have some tea!” he managed to say after taking a deep 
breath. 
 
“A little beer would suit me better, if it is all the same to you, my good sir,” 
said Balin with the white beard. “But I don’t mind some cake—seed-cake, if you 
have any.” 
 
“Lots!” Bilbo found himself answering, to his own surprise; and he found 
himself scuttling off, too, to the cellar to fill a pint beer-mug, and then to a 
pantry to fetch two beautiful round seed-cakes which he had baked that 
afternoon for his after-supper morsel. 
 
When he got back Balin and Dwalin were talking at the table like old friends 
(as a matter of fact they were brothers). Bilbo plumped down the beer and the 
cake in front of them, when loud came a ring at the bell again, and then another 
ring. 
 
“Gandalf for certain this time,” he thought as he puffed along the passage. 
 
But it was not. It was two more dwarves, both with blue hoods, silver belts, and 
yellow beards; and each of them carried a bag of tools and a spade. In they 
hopped, as soon as the door began to open—Bilbo was hardly surprised at all. 
 
“What can I do for you, my dwarves?” he said. 
 
“Kili at your service!” said the one. “And Fili!” added the other; and they 
both swept off their blue hoods and bowed. 
 
“At yours and your family’s!” replied Bilbo, remembering his manners this 
time. 
 
“Dwalin and Balin here already, I see,” said Kili. “Let us join the throng!” 
 
“Throng!” thought Mr. Baggins. “I don’t like the sound of that. I really must 
sit down for a minute and collect my wits, and have a drink.” He had only just 
had a sip—in the corner, while the four dwarves sat round the table, and talked 
about mines and gold and troubles with the goblins, and the depredations of 
dragons, and lots of other things which he did not understand, and did not want 
 
 
 
to, for they sounded much too adventurous—when, ding-dong-a-ling-dang, his 
bell rang again, as if some naughty little hobbit-boy was trying to pull the handle 
off. 
 
“Someone at the door!” he said, blinking. 
 
“Some four, I should say by the sound,” said Fili. “Besides, we saw them 
coming along behind us in the distance.” 
 
The poor little hobbit sat down in the hall and put his head in his hands, and 
wondered what had happened, and what was going to happen, and whether they 
would all stay to supper. Then the bell rang again louder than ever, and he had to 
mn to the door. It was not four after all, it was five. Another dwarf had come 
along while he was wondering in the hall. He had hardly turned the knob, before 
they were all inside, bowing and saying “at your service” one after another. Dori, 
Nori, Ori, Oin, and Gloin were their names; and very soon two purple hoods, a 
grey hood, a brown hood, and a white hood were hanging on the pegs, and off 
they marched with their broad hands stuck in their gold and silver belts to join 
the others. Already it had almost become a throng. Some called for ale, and 
some for porter, and one for coffee, and all of them for cakes; so the hobbit was 
kept very busy for a while. 
 
A big jug of coffee had just been set in the hearth, the seed-cakes were gone, 
and the dwarves were starting on a round of buttered scones, when there came— 
a loud knock. Not a ring, but a hard rat-tat on the hobbit’s beautiful green door. 
Somebody was banging with a stick! 
 
Bilbo rushed along the passage, very angry, and altogether bewildered and 
bewuthered—this was the most awkward Wednesday he ever remembered. He 
pulled open the door with a jerk, and they all fell in, one on top of the other. 
 
More dwarves, four more! And there was Gandalf behind, leaning on his staff 
and laughing. He had made quite a dent on the beautiful door; he had also, by the 
way, knocked out the secret mark that he had put there the morning before. 
 
“Carefully! Carefully!” he said. “It is not like you, Bilbo, to keep friends 
waiting on the mat, and then open the door like a pop-gun! Let me introduce 
Bifur, Bofur, Bombur, and especially Thorin!” 
 
“At your service!” said Bifur, Bofur, and Bombur standing in a row. Then 
they hung up two yellow hoods and a pale green one; and also a sky-blue one 
with a long silver tassel. This last belonged to Thorin, an enormously important 
dwarf, in fact no other than the great Thorin Oakenshield himself, who was not 
at all pleased at falling flat on Bilbo’s mat with Bifur, Bofur, and Bombur on top 
of him. For one thing Bombur was immensely fat and heavy. Thorin indeed was 
 
 
 
very haughty, and said nothing about service; but poor Mr. Baggins said he was 
sorry so many times, that at last he grunted “pray don’t mention it,” and stopped 
frowning. 
 
“Now we are all here!” said Gandalf, looking at the row of thirteen hoods— 
the best detachable party hoods—and his own hat hanging on the pegs. “Quite a 
merry gathering! I hope there is something left for the late-comers to eat and 
drink! What’s that? Tea! No thank you! A little red wine, I think for me.” 
 
“And for me,” said Thorin. 
 
“And raspberry jam and apple-tart,” said Bifur. 
 
“And mince-pies and cheese,” said Bofur. 
 
“And pork-pie and salad,” said Bombur. 
 
“And more cakes—and ale—and coffee, if you don’t mind,” called the other 
dwarves through the door. 
 
“Put on a few eggs, there’s a good fellow!” Gandalf called after him, as the 
hobbit stumped off to the pantries. “And just bring out the cold chicken and 
pickles!” 
 
“Seems to know as much about the inside of my larders as I do myself!” 
thought Mr. Baggins, who was feeling positively flummoxed, and was beginning 
to wonder whether a most wretched adventure had not come right into his house. 
By the time he had got all the bottles and dishes and knives and forks and glasses 
and plates and spoons and things piled up on big trays, he was getting very hot, 
and red in the face, and annoyed. 
 
“Confusticate and bebother these dwarves!” he said aloud. “Why don’t they 
come and lend a hand?” Lo and behold! there stood Balin and Dwalin at the door 
of the kitchen, and Fili and Kili behind them, and before he could say knife they 
had whisked the trays and a couple of small tables into the parlour and set out 
everything afresh. 
 
Gandalf sat at the head of the party with the thirteen dwarves all round: and 
Bilbo sat on a stool at the fireside, nibbling at a biscuit (his appetite was quite 
taken away), and trying to look as if this was all perfectly ordinary and not in the 
least an adventure. The dwarves ate and ate, and talked and talked, and time got 
on. At last they pushed their chairs back, and Bilbo made a move to collect the 
plates and glasses. 
 
“I suppose you will all stay to supper?” he said in his politest unpressing 
tones. 
 
“Of course!” said Thorin. “And after. We shan’t get through the business till 
late, and we must have some music first. Now to clear up!”
Thereupon the twelve dwarves—not Thorin, he was too important, and 
stayed talking to Gandalf—jumped to their feet, and made tall piles of all the 
things. Off they went, not waiting for trays, balancing columns of plates, each 
with a bottle on the top, with one hand, while the hobbit ran after them almost 
squeaking with fright: “please be careful!” and “please, don’t trouble! I can 
manage.” But the dwarves only started to sing: 
 
 
Chip the glasses and crack the plates! 
 
Blunt the knives and bend the forks! 
That’s what Bilbo Baggins hates- 
Smash the bottles and burn the corks! 
 
 
Cut the cloth and tread on the fat! 
 
Pour the milk on the pantry floor! 
Leave the bones on the bedroom mat! 
Splash the wine on every door! 
 
 
Dump the crocks in a boiling bowl; 
 
Pound them up with a thumping pole; 
And when you’ve finished, if any are whole, 
Send them down the hall to roll! 
 
 
That’s what Bilbo Baggins hates! 
 
So, carefully! carefully with the plates! 
 
 
And of course they did none of these dreadful things, and everything was 
cleaned and put away safe as quick as lightning, while the hobbit was turning 
round and round in the middle of the kitchen trying to see what they were doing. 
Then they went back, and found Thorin with his feet on the fender smoking a 
pipe. He was blowing the most enormous smoke-rings, and wherever he told one 
to go, it went—up the chimney, or behind the clock on the mantelpiece, or under 
the table, or round and round the ceiling; but wherever it went it was not quick 
enough to escape Gandalf. Pop! he sent a smaller smoke-ring from his short 
clay-pipe straight through each one of Thorin’s. Then Gandalf’s smoke-ring 
 
 
 
would go green and come back to hover over the wizard’s head. He had a cloud 
of them about him already, and in the dim light it made him look strange and 
sorcerous. Bilbo stood still and watched—he loved smoke-rings—and then he 
blushed to think how proud he had been yesterday morning of the smoke-rings 
he had sent up the wind over The Hill. 
 
“Now for some music!” said Thorin. “Bring out the instruments!” 
 
Kili and Fili rushed for their bags and brought back little fiddles; Dori, Nori, 
and Ori brought out flutes from somewhere inside their coats; Bombur produced 
a drum from the hall; Bifur and Bofur went out too, and came back with clarinets 
that they had left among the walking-sticks. Dwalin and Balin said: “Excuse me, 
 
I left mine in the porch!” “Just bring mine in with you!” said Thorin. They came 
back with viols as big as themselves, and with Thorin’s harp wrapped in a green 
cloth. It was a beautiful golden harp, and when Thorin struck it the music began 
all at once, so sudden and sweet that Bilbo forgot everything else, and was swept 
away into dark lands under strange moons, far over The Water and very far from 
his hobbit-hole under The Hill. 
 
The dark came into the room from the little window that opened in the side of 
The Hill; the firelight flickered—it was April—and still they played on, while 
the shadow of Gandalf’s beard wagged against the wall. 
 
The dark filled all the room, and the fire died down, and the shadows were 
lost, and still they played on. And suddenly first one and then another began to 
sing as they played, deep-throated singing of the dwarves in the deep places of 
their ancient homes; and this is like a fragment of their song, if it can be like 
their song without their music. 
 
 
Far over the misty mountains cold 
To dungeons deep and caverns old 
We must away ere break of day 
To seek the pale enchanted gold. 
 
 
The dwarves of yore made mighty spells, 
While hammers fell like ringing bells 
In places deep, where dark things sleep, 
In hollow halls beneath the fells. 
 
 
 
For ancient king and elvish lord 
There many a gleaming golden hoard 
They shaped and wrought, and light they caught 
To hide in gems on hilt of sword. 
 
 
On silver necklaces they strung 
 
The flowering stars, on crowns they hung 
 
The dragon-fire, in twisted wire 
 
They meshed the light of moon and sun. 
 
 
Far over the misty mountains cold 
To dungeons deep and caverns old 
We must away, ere break of day, 
 
To claim our long-forgotten gold. 
 
 
Goblets they carved there for themselves 
And harps of gold; where no man delves 
There lay they long, and many a song 
Was sung unheard by men or elves. 
 
 
The pines were roaring on the height, 
The winds were moaning in the night. 
The fire was red, it flaming spread; 
 
The trees like torches blazed with light. 
 
 
The bells were ringing in the dale 
And men looked up with faces pale; 
The dragon’s ire more fierce than fire 
Laid low their towers and houses frail. 
 
 
The mountain smoked beneath the moon; 
The dwarves, they heard the tramp of doom. 
They fled their hall to dying fall 
 
 
 
Beneath his feet, beneath the moon. 
 
 
Far over the misty mountains grim 
To dungeons deep and caverns dim 
We must away, ere break of day, 
 
To win our harps and gold from him! 
 
 
As they sang the hobbit felt the love of beautiful things made by hands and 
by cunning and by magic moving through him, a fierce and a jealous love, the 
desire of the hearts of dwarves. Then something Tookish woke up inside him, 
and he wished to go and see the great mountains, and hear the pine-trees and the 
waterfalls, and explore the caves, and wear a sword instead of a walking-stick. 
 
He looked out of the window. The stars were out in a dark sky above the trees. 
 
He thought of the jewels of the dwarves shining in dark caverns. Suddenly in the 
wood beyond The Water a flame leapt up—probably somebody lighting a wood- 
fire—and he thought of plundering dragons settling on his quiet Hill and 
kindling it all to flames. He shuddered; and very quickly he was plain Mr. 
Baggins of Bag-End, Under-Hill, again. 
 
He got up trembling. He had less than half a mind to fetch the lamp, and 
more than half a mind to pretend to, and go and hide behind the beer-barrels in 
the cellar, and not come out again until all the dwarves had gone away. Suddenly 
he found that the music and the singing had stopped, and they were all looking at 
him with eyes shining in the dark. 
 
“Where are you going?” said Thorin, in a tone that seemed to show that he 
guessed both halves of the hobbit’s mind. 
 
“What about a little light?” said Bilbo apologetically. 
 
“We like the dark,” said all the dwarves. “Dark for dark business! There are 
many hours before dawn.” 
 
“Of course! ” said Bilbo, and sat down in a hurry. He missed the stool and sat 
in the fender, knocking over the poker and shovel with a crash. 
 
“Hush!” said Gandalf. “Let Thorin speak!” And this is how Thorin began. 
 
“Gandalf, dwarves and Mr. Baggins! We are met together in the house of our 
friend and fellow conspirator, this most excellent and audacious hobbit—may 
the hair on his toes never fall out! all praise to his wine and ale!—” He paused 
for breath and for a polite remark from the hobbit, but the compliments were 
quite lost on poor Bilbo Baggins, who was wagging his mouth in protest at being 
 
 
 
called audacious and worst of all fellow conspirator, though no noise came out, 
he was so flummoxed. So Thorin went on: 
 
“We are met to discuss our plans, our ways, means, policy and devices. We 
shall soon before the break of day start on our long journey, a journey from 
which some of us, or perhaps all of us (except our friend and counsellor, the 
ingenious wizard Gandalf) may never return. It is a solemn moment. Our object 
is, I take it, well known to us all. To the estimable Mr. Baggins, and perhaps to 
one or two of the younger dwarves (I think I should be right in naming Kili and 
Fili, for instance), the exact situation at the moment may require a little brief 
explanation—” 
 
This was Thorin’s style. He was an important dwarf. If he had been allowed, 
he would probably have gone on like this until he was out of breath, without 
telling any one there anything that was not known already. But he was rudely 
interrupted. Poor Bilbo couldn’t bear it any longer. At may never return he began 
to feel a shriek coming up inside, and very soon it burst out like the whistle of an 
engine coming out of a tunnel. All the dwarves sprang up, knocking over the 
table. Gandalf struck a blue light on the end of his magic staff, and in its 
firework glare the poor little hobbit could be seen kneeling on the hearth-rug, 
shaking like a jelly that was melting. Then he fell flat on the floor, and kept on 
calling out “struck by lightning, struck by lightning!” over and over again; and 
that was all they could get out of him for a long time. So they took him and laid 
him out of the way on the drawing-room sofa with a drink at his elbow, and they 
went back to their dark business. 
 
“Excitable little fellow,” said Gandalf, as they sat down again. “Gets funny 
queer fits, but he is one of the best, one of the best—as fierce as a dragon in a 
pinch.” 
 
If you have ever seen a dragon in a pinch, you will realize that this was only 
poetical exaggeration applied to any hobbit, even to Old Took’s great-grand¬ 
uncle Bullroarer, who was so huge (for a hobbit) that he could ride a horse. He 
charged the ranks of the goblins of Mount Gram in the Battle of the Green 
Fields, and knocked their king Golfimbul’s head clean off with a wooden club. It 
sailed a hundred yards through the air and went down a rabbit-hole, and in this 
way the battle was won and the game of Golf invented at the same moment. 
 
In the meanwhile, however, Bullroarer’s gentler descendant was reviving in 
the drawing-room. After a while and a drink he crept nervously to the door of the 
parlour. This is what he heard, Gloin speaking: “Humph!” (or some snort more 
or less like that). “Will he do, do you think? It is all very well for Gandalf to talk 
 
 
 
about this hobbit being fierce, but one shriek like that in a moment of excitement 
would be enough to wake the dragon and all his relatives, and kill the lot of us. I 
think it sounded more like fright than excitement! In fact, if it had not been for 
the sign on the door, I should have been sure we had come to the wrong house. 
As soon as I clapped eyes on the little fellow bobbing and puffing on the mat, I 
had my doubts. He looks more like a grocer than a burglar!” 
 
Then Mr. Baggins turned the handle and went in. The Took side had won. He 
suddenly felt he would go without bed and breakfast to be thought fierce. As for 
little fellow bobbing on the mat it almost made him really fierce. Many a time 
afterwards the Baggins part regretted what he did now, and he said to himself: 
“Bilbo, you were a fool; you walked right in and put your foot in it.” 
 
“Pardon me,” he said, “if I have overheard words that you were saying. I 
don’t pretend to understand what you are talking about, or your reference to 
burglars, but I think I am right in believing” (this is what he called being on his 
dignity) “that you think I am no good. I will show you. I have no signs on my 
door—it was painted a week ago—, and I am quite sure you have come to the 
wrong house. As soon as I saw your funny faces on the door-step, I had my 
doubts. But treat it as the right one. Tell me what you want done, and I will try it, 
if I have to walk from here to the East of East and fight the wild Were-worms in 
the Last Desert. I had a great-great-great-grand-uncle once, Bullroarer Took, and 
 
“Yes, yes, but that was long ago,” said Gloin. “I was talking about you. And I 
assure you there is a mark on this door—the usual one in the trade, or used to be. 
Burglar wants a good job, plenty of Excitement and reasonable Reward, that’s 
how it is usually read. You can say Expert Treasure-hunter instead of Burglar if 
you like. Some of them do. It’s all the same to us. Gandalf told us that there was 
a man of the sort in these parts looking for a Job at once, and that he had 
arranged for a meeting here this Wednesday tea-time.” 
 
“Of course there is a mark,” said Gandalf. “I put it there myself. For very 
good reasons. You asked me to find the fourteenth man for your expedition, and 
I chose Mr. Baggins. Just let any one say I chose the wrong man or the wrong 
house, and you can stop at thirteen and have all the bad luck you like, or go back 
to digging coal.” 
 
He scowled so angrily at Gloin that the dwarf huddled back in his chair; and 
when Bilbo tried to open his mouth to ask a question, he turned and frowned at 
him and stuck out his bushy eyebrows, till Bilbo shut his mouth tight with a 
snap. “That’s right,” said Gandalf. “Let’s have no more argument. I have chosen 
 
 
 
Mr. Baggins and that ought to be enough for all of you. If I say he is a Burglar, a 
Burglar he is, or will be when the time comes. There is a lot more in him than 
you guess, and a deal more than he has any idea of himself. You may (possibly) 
all live to thank me yet. Now Bilbo, my boy, fetch the lamp, and let’s have a 
little light on this!” 
 
On the table in the light of a big lamp with a red shade he spread a piece of 
parchment rather like a map. 
 
“This was made by Thror, your grandfather, Thorin,” he said in answer to the 
dwarves’ excited questions. “It is a plan of the Mountain.” 
 
“I don’t see that this will help us much,” said Thorin disappointedly after a 
glance. “I remember the Mountain well enough and the lands about it. And I 
know where Mirkwood is, and the Withered Heath where the great dragons 
bred.” 
 
“There is a dragon marked in red on the Mountain,” said Balin, “but it will be 
easy enough to find him without that, if ever we arrive there.” 
 
“There is one point that you haven’t noticed,” said the wizard, “and that is the 
secret entrance. You see that rune on the West side, and the hand pointing to it 
from the other runes? That marks a hidden passage to the Lower Halls.” (Look at 
the map at the beginning of this book, and you will see there the runes.) 
 
“It may have been secret once,” said Thorin, “but how do we know that it is 
secret any longer? Old Smaug has lived there long enough now to find out 
anything there is to know about those caves.” 
 
“He may—but he can’t have used it for years and years.” 
 
“Why?” 
 
“Because it is too small. ‘ Five feet high the door and three may walk abreast ’ 
say the runes, but Smaug could not creep into a hole that size, not even when he 
was a young dragon, certainly not after devouring so many of the dwarves and 
men of Dale.” 
 
“It seems a great big hole to me,” squeaked Bilbo (who had no experience of 
dragons and only of hobbit-holes). He was getting excited and interested again, 
so that he forgot to keep his mouth shut. He loved maps, and in his hall there 
hung a large one of the Country Round with all his favourite walks marked on it 
in red ink. “How could such a large door be kept secret from everybody outside, 
apart from the dragon?” he asked. He was only a little hobbit you must 
remember. 
 
“In lots of ways,” said Gandalf. “But in what way this one has been hidden 
we don’t know without going to see. From what it says on the map I should 
 
 
 
guess there is a closed door which has been made to look exactly like the side of 
the Mountain. That is the usual dwarves’ method—I think that is right, isn’t it?” 
 
“Quite right,” said Thorin. 
 
“Also,” went on Gandalf, “I forgot to mention that with the map went a key, a 
small and curious key. Here it is!” he said, and handed to Thorin a key with a 
long barrel and intricate wards, made of silver. “Keep it safe!” 
 
“Indeed I will,” said Thorin, and he fastened it upon a fine chain that hung 
about his neck and under his jacket. “Now things begin to look more hopeful. 
This news alters them much for the better. So far we have had no clear idea what 
to do. We thought of going East, as quiet and careful as we could, as far as the 
Long Lake. After that the trouble would begin—.” 
 
“A long time before that, if I know anything about the roads East,” 
interrupted Gandalf. 
 
“We might go from there up along the River Running,” went on Thorin 
taking no notice, “and so to the ruins of Dale—the old town in the valley there, 
under the shadow of the Mountain. But we none of us liked the idea of the Eront 
Gate. The river runs right out of it through the great cliff at the South of the 
Mountain, and out of it comes the dragon too—far too often, unless he has 
changed his habits.” 
 
“That would be no good,” said the wizard, “not without a mighty Warrior, 
even a Hero. I tried to find one; but warriors are busy fighting one another in 
distant lands, and in this neighbourhood heroes are scarce, or simply not to be 
found. Swords in these parts are mostly blunt, and axes are used for trees, and 
shields as cradles or dish-covers; and dragons are comfortably far-off (and 
therefore legendary). That is why I settled on burglary —especially when I 
remembered the existence of a Side-door. And here is our little Bilbo Baggins, 
the burglar, the chosen and selected burglar. So now let’s get on and make some 
plans.” 
 
“Very well then,” said Thorin, “supposing the burglar-expert gives us some 
ideas or suggestions.” He turned with mock-politeness to Bilbo. 
 
“First I should like to know a bit more about things,” said he, feeling all 
confused and a bit shaky inside, but so far still Tookishly determined to go on 
with things. “I mean about the gold and the dragon, and all that, and how it got 
there, and who it belongs to, and so on and further.” 
 
“Bless me!” said Thorin, “haven’t you got a map? and didn’t you hear our 
song? and haven’t we been talking about all this for hours?” 
 
“All the same, I should like it all plain and clear,” said he obstinately, putting 
 
 
 
on his business manner (usually reserved for people who tried to borrow money 
off him), and doing his best to appear wise and pmdent and professional and live 
up to Gandalf’s recommendation. “Also I should like to know about risks, out- 
of-pocket expenses, time required and remuneration, and so forth”—by which he 
meant: “What am I going to get out of it? and am I going to come back alive?” 
 
“O very well,” said Thorin. “Long ago in my grandfather Thror’s time our 
family was driven out of the far North, and came back with all their wealth and 
their tools to this Mountain on the map. It had been discovered by my far 
ancestor, Thrain the Old, but now they mined and they tunnelled and they made 
huger halls and greater workshops—and in addition I believe they found a good 
deal of gold and a great many jewels too. Anyway they grew immensely rich and 
famous, and my grandfather was King under the Mountain again, and treated 
with great reverence by the mortal men, who lived to the South, and were 
gradually spreading up the Running River as far as the valley overshadowed by 
the Mountain. They built the merry town of Dale there in those days. Kings used 
to send for our smiths, and reward even the least skillful most richly. Fathers 
would beg us to take their sons as apprentices, and pay us handsomely, 
especially in food-supplies, which we never bothered to grow or find for 
ourselves. Altogether those were good days for us, and the poorest of us had 
money to spend and to lend, and leisure to make beautiful things just for the fun 
of it, not to speak of the most marvellous and magical toys, the like of which is 
not to be found in the world now-a-days. So my grandfather’s halls became full 
of armour and jewels and carvings and cups, and the toy market of Dale was the 
wonder of the North. 
 
“Undoubtedly that was what brought the dragon. Dragons steal gold and 
jewels, you know, from men and elves and dwarves, wherever they can find 
them; and they guard their plunder as long as they live (which is practically for 
ever, unless they are killed), and never enjoy a brass ring of it. Indeed they 
hardly know a good bit of work from a bad, though they usually have a good 
notion of the current market value; and they can’t make a thing for themselves, 
not even mend a little loose scale of their armour. There were lots of dragons in 
the North in those days, and gold was probably getting scarce up there, with the 
dwarves flying south or getting killed, and all the general waste and destruction 
that dragons make going from bad to worse. There was a most specially greedy, 
strong and wicked worm called Smaug. One day he flew up into the air and 
came south. The first we heard of it was a noise like a hurricane coming from the 
North, and the pine-trees on the Mountain creaking and cracking in the wind. 
 
 
 
Some of the dwarves who happened to be outside (I was one luckily—a fine 
adventurous lad in those days, always wandering about, and it saved my life that 
day)—well, from a good way off we saw the dragon settle on our mountain in a 
spout of flame. Then he came down the slopes and when he reached the woods 
they all went up in fire. By that time all the bells were ringing in Dale and the 
warriors were arming. The dwarves rushed out of their great gate; but there was 
the dragon waiting for them. None escaped that way. The river rushed up in 
steam and a fog fell on Dale, and in the fog the dragon came on them and 
destroyed most of the warriors—the usual unhappy story, it was only too 
common in those days. Then he went back and crept in through the Front Gate 
and routed out all the halls, and lanes, and tunnels, alleys, cellars, mansions and 
passages. After that there were no dwarves left alive inside, and he took all their 
wealth for himself. Probably, for that is the dragons’ way, he has piled it all up in 
a great heap far inside, and sleeps on it for a bed. Later he used to crawl out of 
the great gate and come by night to Dale, and carry away people, especially 
maidens, to eat, until Dale was ruined, and all the people dead or gone. What 
goes on there now I don’t know for certain, but I don’t suppose any one lives 
nearer to the Mountain than the far edge of the Long Lake now-a-days. 
 
“The few of us that were well outside sat and wept in hiding, and cursed 
Smaug; and there we were unexpectedly joined by my father and my grandfather 
with singed beards. They looked very grim but they said very little. When I 
asked how they had got away, they told me to hold my tongue, and said that one 
day in the proper time I should know. After that we went away, and we have had 
to earn our livings as best we could up and down the lands, often enough sinking 
as low as blacksmith-work or even coalmining. But we have never forgotten our 
stolen treasure. And even now, when I will allow we have a good bit laid by and 
are not so badly off”—here Thorin stroked the gold chain round his neck—“we 
still mean to get it back, and to bring our curses home to Smaug—if we can. 
 
“I have often wondered about my father’s and my grandfather’s escape. I see 
now they must have had a private Side-door which only they knew about. But 
apparently they made a map, and I should like to know how Gandalf got hold of 
it, and why it did not come down to me, the rightful heir.” 
 
“I did not 'get hold of it,’ I was given it,” said the wizard. “Your grandfather 
Thror was killed, you remember, in the mines of Moria by Azog the Goblin .” 
 
“Curse his name, yes,” said Thorin. 
 
“And Thrain your father went away on the twenty-first of April, a hundred 
years ago last Thursday, and has never been seen by you since—” 
 
 
 
 
“True, true,” said Thorin. 
 
“Well, your father gave me this to give to you; and if I have chosen my own 
time and way for handing it over, you can hardly blame me, considering the 
trouble I had to find you. Your father could not remember his own name when he 
gave me the paper, and he never told me yours; so on the whole I think I ought to 
be praised and thanked! Here it is,” said he handing the map to Thorin. 
 
“I don’t understand,” said Thorin, and Bilbo felt he would have liked to say 
the same. The explanation did not seem to explain.
`
