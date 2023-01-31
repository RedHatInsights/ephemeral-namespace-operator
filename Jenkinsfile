pipeline {
    agent { label 'insights' }
    stages {
        stage('pr_check') {
            steps {
                sh '''
                    . ./pr_check.sh
                '''
            }
        }
    }
}
