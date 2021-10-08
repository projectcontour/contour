Refactors the LeaderElection into cmd/contour/contour.go to allow for identifying
if a leader has been elected or not to set proper status on the ContourConfiguration
CRD. If the instance of Contour running is not the leader, leader election is disabled, 
or the ContourConfiguration is not being used, then setting status never happens.