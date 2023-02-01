pipeline {
    agent { label 'insights' }
    stages {
        stage('init') {
            steps {
                sh 'env'
            }
        }
        stage('pr_check') {
            steps {
                sh '''
                    . ./pr_check.sh
                '''
            }
        }
    }
}
